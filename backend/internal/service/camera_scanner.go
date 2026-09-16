package service

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nvr/backend/internal/domain"
	"github.com/rs/zerolog/log"
)

// credentialPair — пара логин/пароль для перебора
type credentialPair struct {
	Username string
	Password string
}

// defaultCredentialList — список учётных данных для перебора при сканировании
var defaultCredentialList = []credentialPair{
	{"root", "96811621q"}, // OpenIPC (основной)
	{"admin", "admin"},    // Hikvision/Dahua/OpenIPC
	{"admin", "12345"},    // Hikvision альтернативный
	{"admin", "123456"},   // Hikvision/Dahua
	{"admin", ""},         // без пароля
	{"root", "admin"},     // редко, но бывает
}

// CameraScanner — мультивендорный сканер IP-камер
type CameraScanner struct {
	client   *http.Client
	arpCache map[string]string
	arpMu    sync.Mutex
}

func NewCameraScanner() *CameraScanner {
	s := &CameraScanner{
		client: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
	s.loadArpTable()
	return s
}

// loadArpTable читает ARP-таблицу для получения MAC-адресов
func (s *CameraScanner) loadArpTable() {
	s.arpMu.Lock()
	defer s.arpMu.Unlock()

	s.arpCache = make(map[string]string)

	// Пробуем ip neigh (Linux)
	out, err := exec.Command("ip", "neigh").Output()
	if err != nil {
		out, err = exec.Command("arp", "-a").Output()
		if err != nil {
			return
		}
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Формат ip neigh: IP dev IFACE lladdr MAC ...
		if len(fields) >= 5 && fields[1] == "dev" && fields[3] == "lladdr" {
			ip := fields[0]
			mac := strings.ToLower(fields[4])
			if strings.Count(mac, ":") == 5 && len(mac) == 17 {
				s.arpCache[ip] = mac
			}
			continue
		}
		// Формат arp -a: ? (IP) at MAC [ether] on IFACE
		if len(fields) >= 4 && fields[2] == "at" {
			mac := strings.ToLower(fields[3])
			if strings.Count(mac, ":") != 5 {
				continue
			}
			ipCandidate := ""
			// IP в скобках — обычно fields[1] = "(192.168.1.38)"
			for _, f := range fields {
				if strings.HasPrefix(f, "(") && strings.HasSuffix(f, ")") {
					ipCandidate = strings.Trim(f, "()")
					break
				}
			}
			if ipCandidate != "" && strings.Count(ipCandidate, ".") == 3 {
				s.arpCache[ipCandidate] = mac
			}
		}
	}
	log.Info().Int("arp_entries", len(s.arpCache)).Msg("ARP table loaded")
}

func (s *CameraScanner) getMAC(ip string) string {
	s.arpMu.Lock()
	defer s.arpMu.Unlock()
	return s.arpCache[ip]
}

// Scan сканирует подсеть и возвращает найденные OpenIPC-камеры
func (s *CameraScanner) Scan(ctx context.Context, req domain.ScanRequest) (*domain.ScanResult, error) {
	_, ipnet, err := net.ParseCIDR(req.Subnet)
	if err != nil {
		return nil, fmt.Errorf("invalid subnet: %w", err)
	}

	// Перезагружаем ARP-таблицу перед сканом
	s.loadArpTable()

	// Получаем все IP в подсети
	ips := make([]net.IP, 0, 254)
	start := make(net.IP, len(ipnet.IP))
	copy(start, ipnet.IP)
	// Пропускаем network address (не сканируем .0)
	inc(start)

	for ip := start; ipnet.Contains(ip); inc(ip) {
		// Пропускаем broadcast
		if isBroadcast(ip, ipnet) {
			continue
		}
		ipCopy := make(net.IP, len(ip))
		copy(ipCopy, ip)
		ips = append(ips, ipCopy)

		// Защита от бесконечного цикла
		if len(ips) > 65536 {
			break
		}
	}

	log.Info().Str("subnet", req.Subnet).Int("ips", len(ips)).Msg("starting camera scan")

	result := &domain.ScanResult{
		Subnet: req.Subnet,
		Total:  len(ips),
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	// Параллельное сканирование до 100 IP одновременно
	sem := make(chan struct{}, 100)

	for _, ip := range ips {
		wg.Add(1)
		go func(ip net.IP) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			cam := s.probeCamera(ctx, ip.String(), req.Username, req.Password)
			if cam != nil {
				mu.Lock()
				result.Cameras = append(result.Cameras, *cam)
				mu.Unlock()
			}
		}(ip)
	}

	wg.Wait()
	result.Found = len(result.Cameras)

	log.Info().Int("found", result.Found).Str("subnet", req.Subnet).Msg("scan complete")
	return result, nil
}

// probeCamera пробует все доступные протоколы с перебором учётных данных.
func (s *CameraScanner) probeCamera(ctx context.Context, ip, userHint, passHint string) *domain.DiscoveredCamera {
	// --- Быстрая проверка: открыт ли RTSP-порт 554 ---
	if !s.checkTCP(ip, 554) {
		// Без RTSP-порта это точно не камера (проверяем всё же HTTP на всякий случай)
		if !s.checkTCP(ip, 80) {
			return nil
		}
	}

	// Формируем список кредов для перебора:
	// 1) если юзер явно указал логин/пароль — пробуем их первыми
	// 2) затем стандартный список
	creds := make([]credentialPair, 0, len(defaultCredentialList)+1)
	if userHint != "" {
		creds = append(creds, credentialPair{userHint, passHint})
	}
	for _, c := range defaultCredentialList {
		// Не дублируем если юзер уже указал такие же
		if userHint != "" && c.Username == userHint && c.Password == passHint {
			continue
		}
		creds = append(creds, c)
	}

	// Цепочка проберов: для каждых кредов пробуем все вендорные API.
	// Найдя подходящие креды — сохраняем и используем дальше.
	for _, cred := range creds {
		// 1. OpenIPC / Majestic API (самый информативный)
		if cam := s.probeMajestic(ctx, ip, cred.Username, cred.Password); cam != nil {
			return cam
		}

		// 2. Hikvision ISAPI
		if cam := s.probeHikvision(ctx, ip, cred.Username, cred.Password); cam != nil {
			return cam
		}

		// 3. Dahua CGI
		if cam := s.probeDahua(ctx, ip, cred.Username, cred.Password); cam != nil {
			return cam
		}

		// 4. Универсальный ONVIF (тяжёлый, только если порт 80/8080 открыт)
		// ONVIF использует WS-Security + SOAP — оставим как fallback
	}

	// 5. Fallback: если ни один HTTP-API не ответил, но RTSP открыт —
	//    создаём generic-запись с RTSP URL
	if s.checkTCP(ip, 554) {
		return &domain.DiscoveredCamera{
			IP:         ip,
			Vendor:     "generic",
			Online:     true,
			MainStream: fmt.Sprintf("rtsp://%s:554/stream=0", ip),
			SubStream:  fmt.Sprintf("rtsp://%s:554/stream=1", ip),
		}
	}

	return nil
}

// checkTCP проверяет доступность TCP-порта
func (s *CameraScanner) checkTCP(ip string, port int) bool {
	addr := net.JoinHostPort(ip, fmt.Sprintf("%d", port))
	conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// =========================================================================
// OpenIPC / Majestic API
// =========================================================================

func (s *CameraScanner) probeMajestic(ctx context.Context, ip, username, password string) *domain.DiscoveredCamera {
	url := fmt.Sprintf("http://%s/api/v1/config.json", ip)
	config, ok := s.httpGetJSON(ctx, url, username, password)
	if !ok || config == nil {
		return nil
	}

	// Проверяем маркер Majestic
	if _, hasSystem := config["system"]; !hasSystem {
		return nil
	}

	cam := &domain.DiscoveredCamera{
		IP:       ip,
		Vendor:   "openipc",
		Online:   true,
		MAC:      s.getMAC(ip),
		Username: username,
		Password: password,
	}

	// Модель (сенсор)
	if isp, ok := config["isp"].(map[string]interface{}); ok {
		if sensorPath, ok := isp["sensorConfig"].(string); ok {
			base := filepath.Base(sensorPath)
			cam.Model = strings.TrimSuffix(base, filepath.Ext(base))
		}
	}

	// Физический IP
	if v, ok := config["network"].(map[string]interface{}); ok {
		if netIP, ok := v["ip"].(string); ok {
			cam.IP = netIP
		}
	}

	// Прошивка
	parts := []string{}
	if v0, ok := config["video0"].(map[string]interface{}); ok {
		if size, ok := v0["size"].(string); ok {
			parts = append(parts, size)
		}
	}
	if v1, ok := config["video1"].(map[string]interface{}); ok {
		if size, ok := v1["size"].(string); ok {
			parts = append(parts, "sub:"+size)
		}
	}
	if cam.Model != "" {
		cam.Firmware = cam.Model
		if len(parts) > 0 {
			cam.Firmware += " " + strings.Join(parts, " ")
		}
	} else if len(parts) > 0 {
		cam.Firmware = strings.Join(parts, " ")
	}

	cam.MainStream = fmt.Sprintf("rtsp://%s/stream=0", ip)
	cam.SubStream = fmt.Sprintf("rtsp://%s/stream=1", ip)
	cam.Snapshot = fmt.Sprintf("http://%s/image.jpg", ip)

	return cam
}

// =========================================================================
// Hikvision ISAPI
// =========================================================================

type isapiDeviceInfo struct {
	XMLName         xml.Name `xml:"DeviceInfo"`
	DeviceName      string   `xml:"deviceName"`
	DeviceID        string   `xml:"deviceID"`
	FirmwareVersion string   `xml:"firmwareVersion"`
	Model           string   `xml:"model"`
	SerialNumber    string   `xml:"serialNumber"`
	MacAddress      string   `xml:"macAddress"`
	Manufacturer    string   `xml:"manufacturer"`
}

func (s *CameraScanner) probeHikvision(ctx context.Context, ip, username, password string) *domain.DiscoveredCamera {
	url := fmt.Sprintf("http://%s/ISAPI/System/deviceInfo", ip)
	body, ok := s.httpGetXML(ctx, url, username, password)
	if !ok || body == nil {
		return nil
	}

	var info isapiDeviceInfo
	if err := xml.Unmarshal(body, &info); err != nil {
		return nil
	}

	// Проверяем что это реально Hikvision (model/manufacturer содержат Hikvision)
	if info.DeviceName == "" && info.Model == "" {
		return nil
	}
	lower := strings.ToLower(info.Manufacturer + info.DeviceName + info.Model)
	if !strings.Contains(lower, "hikvision") && !strings.Contains(lower, "hik") && !strings.Contains(lower, "ds-") {
		// Может быть и другой ISAPI-совместимый вендор — всё равно принимаем
	}

	cam := &domain.DiscoveredCamera{
		IP:       ip,
		Vendor:   "hikvision",
		Model:    info.Model,
		Firmware: info.FirmwareVersion,
		Online:   true,
		Username: username,
		Password: password,
	}

	if info.MacAddress != "" {
		cam.MAC = strings.ToLower(info.MacAddress)
	}
	if cam.MAC == "" {
		cam.MAC = s.getMAC(ip)
	}

	// RTSP URL Hikvision: rtsp://ip:554/Streaming/Channels/101 (main), /102 (sub)
	cam.MainStream = fmt.Sprintf("rtsp://%s:554/Streaming/Channels/101", ip)
	cam.SubStream = fmt.Sprintf("rtsp://%s:554/Streaming/Channels/102", ip)
	cam.Snapshot = fmt.Sprintf("http://%s/ISAPI/Streaming/channels/101/picture", ip)

	return cam
}

// =========================================================================
// Dahua CGI
// =========================================================================

func (s *CameraScanner) probeDahua(ctx context.Context, ip, username, password string) *domain.DiscoveredCamera {
	// Dahua отвечает JSON'ом, структура: { "params": { "magicBox": { ... } } }
	url := fmt.Sprintf("http://%s/cgi-bin/magicBox.cgi?action=getSystemInfo", ip)
	body, ok := s.httpGetJSON(ctx, url, username, password)
	if !ok || body == nil {
		// Пробуем альтернативный эндпоинт
		url = fmt.Sprintf("http://%s/cgi-bin/magicBox.cgi?action=getDeviceType", ip)
		body, ok = s.httpGetJSON(ctx, url, username, password)
		if !ok || body == nil {
			return nil
		}
	}

	// Dahua может обернуть ответ в params или deviceType корень
	root := body
	if params, has := body["params"].(map[string]interface{}); has {
		root = params
	}
	magicBox, _ := root["magicBox"].(map[string]interface{})
	if magicBox == nil {
		magicBox, _ = root["deviceType"].(map[string]interface{})
	}
	if magicBox == nil && len(root) > 0 {
		magicBox = root
	}

	// Извлекаем поля
	model := strVal(magicBox, "deviceType") // или из корня
	if model == "" {
		model = strVal(magicBox, "model")
	}

	cam := &domain.DiscoveredCamera{
		IP:       ip,
		Vendor:   "dahua",
		Model:    model,
		Firmware: strVal(magicBox, "firmwareVersion"),
		MAC:      s.getMAC(ip),
		Online:   true,
		Username: username,
		Password: password,
	}

	if sn := strVal(magicBox, "serial"); sn != "" && cam.MAC == "" {
		// Может содержать MAC
		sn = strings.ToLower(strings.TrimSpace(sn))
		if strings.Count(sn, ":") == 5 {
			cam.MAC = sn
		}
	}

	// RTSP URL Dahua: rtsp://ip:554/cam/realmonitor?channel=1&subtype=0
	cam.MainStream = fmt.Sprintf("rtsp://%s:554/cam/realmonitor?channel=1&subtype=0", ip)
	cam.SubStream = fmt.Sprintf("rtsp://%s:554/cam/realmonitor?channel=1&subtype=1", ip)
	cam.Snapshot = fmt.Sprintf("http://%s/cgi-bin/snapshot.cgi?channel=1", ip)

	return cam
}

// =========================================================================
// HTTP helpers
// =========================================================================

// httpGetJSON делает GET с Basic Auth и парсит JSON-ответ.
func (s *CameraScanner) httpGetJSON(ctx context.Context, url, username, password string) (map[string]interface{}, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, false
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, false
	}

	return result, true
}

// httpGetXML делает GET и возвращает сырой XML-тело.
func (s *CameraScanner) httpGetXML(ctx context.Context, url, username, password string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, false
	}
	if username != "" {
		req.SetBasicAuth(username, password)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 128*1024))
	if err != nil {
		return nil, false
	}

	// Проверяем что похоже на XML
	if len(body) == 0 || body[0] != '<' {
		return nil, false
	}

	return body, true
}

// ProbeSingle проверяет одну конкретную камеру по IP.
func (s *CameraScanner) ProbeSingle(ctx context.Context, ip, username, password string) (*domain.DiscoveredCamera, error) {
	cam := s.probeCamera(ctx, ip, username, password)
	if cam == nil {
		return nil, fmt.Errorf("no camera found at %s", ip)
	}
	return cam, nil
}

func strVal(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isBroadcast(ip net.IP, n *net.IPNet) bool {
	mask := n.Mask
	bcast := make(net.IP, len(ip))
	for i := range ip {
		bcast[i] = ip[i] | ^mask[i]
	}
	return ip.Equal(bcast)
}

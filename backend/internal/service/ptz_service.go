package service

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// PTZServiceClient — управление поворотными камерами (PTZ) по ONVIF.
//
// ONVIF выбран как основной протокол: он поддерживается большинством
// PTZ-камер (Vivotek, Hikvision, Dahua, Axis) и не требует знания
// вендорских CGI. Профиль и токен PTZ-конфигурации определяются
// автоматически и кешируются, чтобы не дёргать камеру на каждую команду.
type PTZServiceClient struct {
	client *http.Client

	mu      sync.Mutex
	devices map[string]*ptzDevice // ip -> найденные токены
}

// ptzDevice — закешированные токены ONVIF для конкретной камеры.
type ptzDevice struct {
	MediaProfileToken string // обычно Profile1
	PTZConfigToken    string
	checkedAt         time.Time
}

// PTZStatus — текущее положение камеры и её возможности.
type PTZStatus struct {
	Pan      float64 `json:"pan"`
	Tilt     float64 `json:"tilt"`
	Zoom     float64 `json:"zoom"`
	Supports bool    `json:"supports_ptz"`
}

func NewPTZServiceClient() *PTZServiceClient {
	return &PTZServiceClient{
		client:  &http.Client{Timeout: 8 * time.Second},
		devices: make(map[string]*ptzDevice),
	}
}

// SOAP-обёртка, общая для всех запросов ONVIF.
const soapEnvelope = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>%s</s:Body>
</s:Envelope>`

// call выполняет ONVIF-запрос и возвращает тело ответа.
func (p *PTZServiceClient) call(ctx context.Context, ip, username, password, service, action, body string) ([]byte, error) {
	if ip == "" {
		return nil, fmt.Errorf("camera has no IP address")
	}

	url := fmt.Sprintf("http://%s/onvif/%s", ip, service)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url,
		bytes.NewReader([]byte(fmt.Sprintf(soapEnvelope, body))))
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(username, password)
	req.Header.Set("Content-Type",
		fmt.Sprintf(`application/soap+xml; charset=utf-8; action="%s"`, action))

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return data, fmt.Errorf("onvif returned status %d", resp.StatusCode)
	}
	// ONVIF сообщает об ошибке внутри тела с кодом 200.
	if bytes.Contains(data, []byte("env:Fault")) {
		return data, fmt.Errorf("onvif fault: %s", extractFaultReason(data))
	}
	return data, nil
}

// resolveTokens определяет токен медиапрофиля и PTZ-конфигурации.
// Результат кешируется на час: токены на камере практически не меняются.
func (p *PTZServiceClient) resolveTokens(ctx context.Context, ip, username, password string) (*ptzDevice, error) {
	p.mu.Lock()
	if d, ok := p.devices[ip]; ok && time.Since(d.checkedAt) < time.Hour {
		p.mu.Unlock()
		return d, nil
	}
	p.mu.Unlock()

	// 1. Медиапрофиль
	body, err := p.call(ctx, ip, username, password, "media_service",
		"http://www.onvif.org/ver10/media/wsdl/GetProfiles",
		`<GetProfiles xmlns="http://www.onvif.org/ver10/media/wsdl"/>`)
	if err != nil {
		return nil, fmt.Errorf("get profiles: %w", err)
	}
	// Первый токен из ответа, кроме токенов конфигураций (они содержат "Configuration").
	profileToken := firstTokenMatching(body, func(t string) bool {
		return !strings.Contains(t, "Configuration") && t != "0"
	})
	if profileToken == "" {
		profileToken = "Profile1" // распространённое значение по умолчанию
	}

	// 2. Токен PTZ-конфигурации
	ptzToken := ""
	if data, err := p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/GetConfigurations",
		`<GetConfigurations xmlns="http://www.onvif.org/ver20/ptz/wsdl"/>`); err == nil {
		ptzToken = firstTokenMatching(data, func(t string) bool {
			return strings.Contains(t, "ptz") || strings.Contains(t, "PTZ")
		})
	}

	dev := &ptzDevice{
		MediaProfileToken: profileToken,
		PTZConfigToken:    ptzToken,
		checkedAt:         time.Now(),
	}

	p.mu.Lock()
	p.devices[ip] = dev
	p.mu.Unlock()

	log.Debug().Str("ip", ip).Str("profile", profileToken).Str("ptz_config", ptzToken).
		Msg("ptz tokens resolved")
	return dev, nil
}

// GetStatus возвращает текущее положение камеры.
func (p *PTZServiceClient) GetStatus(ctx context.Context, ip, username, password string) (*PTZStatus, error) {
	dev, err := p.resolveTokens(ctx, ip, username, password)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(
		`<GetStatus xmlns="http://www.onvif.org/ver20/ptz/wsdl"><ProfileToken>%s</ProfileToken></GetStatus>`,
		dev.MediaProfileToken)
	data, err := p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/GetStatus", body)
	if err != nil {
		return nil, err
	}

	st := &PTZStatus{Supports: true}
	st.Pan = parseAttrFloat(data, "PanTilt", "x")
	st.Tilt = parseAttrFloat(data, "PanTilt", "y")
	st.Zoom = parseAttrFloat(data, "Zoom", "x")
	return st, nil
}

// Move запускает непрерывное движение и через duration останавливает его.
// Это самый переносимый способ: он не зависит от поддержки AbsoluteMove.
func (p *PTZServiceClient) Move(ctx context.Context, ip, username, password string, pan, tilt, zoom float64, duration time.Duration) error {
	dev, err := p.resolveTokens(ctx, ip, username, password)
	if err != nil {
		return err
	}

	velocity := fmt.Sprintf(`<PanTilt x="%.3f" y="%.3f" xmlns="http://www.onvif.org/ver10/schema"/>`, pan, tilt)
	if zoom != 0 {
		velocity += fmt.Sprintf(`<Zoom x="%.3f" xmlns="http://www.onvif.org/ver10/schema"/>`, zoom)
	}

	body := fmt.Sprintf(
		`<ContinuousMove xmlns="http://www.onvif.org/ver20/ptz/wsdl">
		   <ProfileToken>%s</ProfileToken>
		   <Velocity>%s</Velocity>
		 </ContinuousMove>`,
		dev.MediaProfileToken, velocity)

	if _, err := p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/ContinuousMove", body); err != nil {
		return err
	}

	// Останавливаем движение по таймеру: ContinuousMove сам не завершается.
	time.AfterFunc(duration, func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := p.Stop(stopCtx, ip, username, password); err != nil {
			log.Debug().Err(err).Str("ip", ip).Msg("ptz auto-stop failed")
		}
	})
	return nil
}

// Stop останавливает движение по всем осям.
func (p *PTZServiceClient) Stop(ctx context.Context, ip, username, password string) error {
	dev, err := p.resolveTokens(ctx, ip, username, password)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(
		`<Stop xmlns="http://www.onvif.org/ver20/ptz/wsdl">
		   <ProfileToken>%s</ProfileToken><PanTilt>true</PanTilt><Zoom>true</Zoom>
		 </Stop>`,
		dev.MediaProfileToken)

	_, err = p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/Stop", body)
	return err
}

// GotoPreset переходит к сохранённой позиции.
func (p *PTZServiceClient) GotoPreset(ctx context.Context, ip, username, password, presetToken string) error {
	dev, err := p.resolveTokens(ctx, ip, username, password)
	if err != nil {
		return err
	}

	body := fmt.Sprintf(
		`<GotoPreset xmlns="http://www.onvif.org/ver20/ptz/wsdl">
		   <ProfileToken>%s</ProfileToken><PresetToken>%s</PresetToken>
		 </GotoPreset>`,
		dev.MediaProfileToken, presetToken)

	_, err = p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/GotoPreset", body)
	return err
}

// GetPresets возвращает список сохранённых позиций (может быть пустым).
func (p *PTZServiceClient) GetPresets(ctx context.Context, ip, username, password string) ([]PTZPreset, error) {
	dev, err := p.resolveTokens(ctx, ip, username, password)
	if err != nil {
		return nil, err
	}

	body := fmt.Sprintf(
		`<GetPresets xmlns="http://www.onvif.org/ver20/ptz/wsdl"><ProfileToken>%s</ProfileToken></GetPresets>`,
		dev.MediaProfileToken)
	data, err := p.call(ctx, ip, username, password, "ptz_service",
		"http://www.onvif.org/ver20/ptz/wsdl/GetPresets", body)
	if err != nil {
		return nil, err
	}

	presets := []PTZPreset{}
	for _, m := range rePreset.FindAllSubmatch(data, -1) {
		presets = append(presets, PTZPreset{
			Token: string(m[1]),
			Name:  string(m[2]),
		})
	}
	return presets, nil
}

// PTZPreset — сохранённая позиция камеры.
type PTZPreset struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// --- вспомогательные функции разбора XML ---

var (
	reToken  = regexp.MustCompile(`token="([^"]+)"`)
	rePreset = regexp.MustCompile(`(?s)<(?:tt:)?Preset\s+token="([^"]*)"[^>]*>.*?<(?:tt:)?Name>([^<]*)</`)
)

// firstTokenMatching возвращает первый token="..." из ответа, прошедший фильтр.
func firstTokenMatching(data []byte, accept func(string) bool) string {
	for _, m := range reToken.FindAllSubmatch(data, -1) {
		if t := string(m[1]); accept(t) {
			return t
		}
	}
	return ""
}

// parseAttrFloat извлекает значение атрибута у элемента, например
// из <tt:PanTilt x="-0.5" y="0.2"/> берёт x.
func parseAttrFloat(data []byte, element, attr string) float64 {
	re := regexp.MustCompile(`<(?:[a-zA-Z0-9]+:)?` + element + `\s[^>]*` + attr + `="([-0-9.eE+]+)"`)
	m := re.FindSubmatch(data)
	if m == nil {
		return 0
	}
	v, err := strconv.ParseFloat(string(m[1]), 64)
	if err != nil {
		return 0
	}
	return v
}

// extractFaultReason вытаскивает текст ошибки из SOAP Fault.
func extractFaultReason(data []byte) string {
	re := regexp.MustCompile(`(?s)<(?:[a-zA-Z0-9]+:)?Text[^>]*>([^<]+)<`)
	if m := re.FindSubmatch(data); m != nil {
		return string(m[1])
	}
	return "unknown fault"
}

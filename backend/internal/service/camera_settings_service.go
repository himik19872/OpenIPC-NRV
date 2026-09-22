package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/nvr/backend/internal/domain"
	"github.com/nvr/backend/internal/repository/postgres"
	"github.com/rs/zerolog/log"
)

// Управление камерой OpenIPC через HTTP API вместо SSH.
//
// Раньше настройки камеры менялись только по SSH: нужно было держать
// root-пароль, в контейнере требовался sshpass, а каждая правка сводилась
// к ручной правке /etc/majestic.yaml. У Majestic есть HTTP API, покрывающий
// то же самое: чтение конфигурации, запись изменений, перезапуск.
//
// Что это даёт: настройки меняются без доступа к shell камеры, изменения
// применяются штатным механизмом прошивки, а не правкой файлов в обход.

// CameraSettingsService читает и меняет настройки камеры через Majestic API.
type CameraSettingsService struct {
	repo *postgres.CameraRepo
	// ssh остаётся резервом: на части камер стоит старая сборка OpenIPC
	// без Majestic, и API там недоступен вовсе.
	ssh *CameraSSH
}

func NewCameraSettingsService(repo *postgres.CameraRepo) *CameraSettingsService {
	return &CameraSettingsService{repo: repo, ssh: NewCameraSSH()}
}

// CameraSettings — настройки камеры, доступные для изменения через API.
//
// Набор намеренно ограничен тем, что безопасно менять на живой камере:
// параметры кодирования, изображения, ночного режима и OSD. Сетевые
// настройки и разделы вроде watchdog сюда не входят — ошибка в них
// оставит камеру недоступной.
//
// Поля — указатели. Это принципиально: интерфейс присылает только те
// настройки, которые оператор тронул, и nil означает «не менять».
// С обычными типами ноль был бы неотличим от «выключить», и правка
// одной строки OSD затирала бы ночной режим и кодирование.
type CameraSettings struct {
	// Видео
	MainFPS     *int    `json:"main_fps,omitempty"`
	MainBitrate *int    `json:"main_bitrate,omitempty"`
	MainSize    *string `json:"main_size,omitempty"`
	MainCodec   *string `json:"main_codec,omitempty"`
	SubFPS      *int    `json:"sub_fps,omitempty"`
	SubBitrate  *int    `json:"sub_bitrate,omitempty"`
	SubSize     *string `json:"sub_size,omitempty"`
	SubEnabled  *bool   `json:"sub_enabled,omitempty"`

	// Изображение: значения в диапазоне 0..100, 50 — середина.
	Mirror     *bool `json:"mirror,omitempty"`
	Flip       *bool `json:"flip,omitempty"`
	Contrast   *int  `json:"contrast,omitempty"`
	Hue        *int  `json:"hue,omitempty"`
	Saturation *int  `json:"saturation,omitempty"`
	Luminance  *int  `json:"luminance,omitempty"`

	// Ночной режим
	NightMode   *NightMode `json:"night_mode,omitempty"`
	AntiFlicker *string    `json:"anti_flicker,omitempty"`

	// OSD (надпись поверх видео)
	OSDEnabled  *bool   `json:"osd_enabled,omitempty"`
	OSDTemplate *string `json:"osd_template,omitempty"`
	OSDSize     *string `json:"osd_size,omitempty"`
	OSDPosX     *int    `json:"osd_pos_x,omitempty"`
	OSDPosY     *int    `json:"osd_pos_y,omitempty"`
	OSDBgAlpha  *int    `json:"osd_bg_alpha,omitempty"`
	OSDOutline  *bool   `json:"osd_outline,omitempty"`
}

// NightMode описывает режим съёмки в темноте.
type NightMode struct {
	// ColorToGray — переводить изображение в ч/б ночью.
	ColorToGray *bool `json:"color_to_gray,omitempty"`
	// IRCut: auto, on, off — управление ИК-фильтром.
	IRCut *string `json:"ir_cut,omitempty"`
	// AutoNightDelay / AutoDayDelay — задержки переключения, секунды.
	AutoNightDelay *int `json:"auto_night_delay,omitempty"`
	AutoDayDelay   *int `json:"auto_day_delay,omitempty"`
	// Backlight — режим компенсации задней засветки: auto, off, ...
	Backlight *string `json:"backlight,omitempty"`
}

// CameraSettingsView — настройки вместе со сведениями об устройстве.
type CameraSettingsView struct {
	Settings       CameraSettings      `json:"settings"`
	Device         *MajesticDeviceInfo `json:"device,omitempty"`
	ReadOnly       bool                `json:"read_only"`
	ReadOnlyReason string              `json:"read_only_reason,omitempty"`
}

// GetSettings читает текущие настройки камеры.
func (s *CameraSettingsService) GetSettings(ctx context.Context, id uuid.UUID) (*CameraSettingsView, error) {
	cam, client, err := s.clientFor(ctx, id)
	if err != nil {
		return nil, err
	}

	cfg, err := client.GetConfig(ctx)
	if err != nil {
		return nil, s.explain(cam, err)
	}

	view := &CameraSettingsView{Settings: parseCameraSettings(cfg)}

	// Сведения об устройстве не критичны: если страница не открылась,
	// настройки всё равно показываем.
	if info, err := client.GetDeviceInfo(ctx); err == nil {
		view.Device = info
	} else {
		log.Debug().Str("камера", cam.IP).Err(err).Msg("не удалось прочитать сведения о камере")
	}

	return view, nil
}

// UpdateSettings записывает настройки камеры.
//
// Прошивка принимает частичный патч: в теле запроса уходят только те
// разделы и поля, которые нужно поменять (проверено на камерах проекта).
// Поэтому в патч попадают исключительно те поля, которые пришли в запросе, —
// так правка строки OSD не перезапишет заодно ночной режим и кодирование.
func (s *CameraSettingsService) UpdateSettings(ctx context.Context, id uuid.UUID, patch CameraSettings) error {
	cam, client, err := s.clientFor(ctx, id)
	if err != nil {
		return err
	}

	if err := validateCameraSettings(patch); err != nil {
		return err
	}

	// Отправляем только то, что просили изменить: текущие значения
	// не читаем, лишних записей на камеру не делаем.
	majesticPatch := buildSettingsPatch(patch)
	if len(majesticPatch) == 0 {
		log.Info().Str("камера", cam.IP).Msg("в запросе нет настроек для изменения")
		return nil
	}

	if err := client.SetConfig(ctx, majesticPatch); err != nil {
		return fmt.Errorf("записать настройки: %w", err)
	}

	log.Info().Str("камера", cam.IP).Interface("разделы", keysOf(majesticPatch)).
		Msg("настройки камеры обновлены")
	return nil
}

// Restart перезапускает камеру: через API, а для старых сборок — по SSH.
func (s *CameraSettingsService) Restart(ctx context.Context, id uuid.UUID) (*CommandResult, error) {
	cam, client, err := s.clientFor(ctx, id)
	if err != nil {
		return nil, err
	}

	username, password := credentialsFromSettings(cam.Settings)

	if err := client.Restart(ctx); err != nil {
		// Старая сборка без Majestic — пробуем SSH, если он доступен.
		if username == "" {
			return &CommandResult{Command: "restart", Error: err.Error()}, err
		}
		log.Info().Str("камера", cam.IP).Msg("перезапуск через API не удался, пробуем SSH")
		return s.ssh.RestartMajestic(ctx, cam.IP, username, password)
	}

	return &CommandResult{Command: "restart camera", Success: true}, nil
}

// clientFor достаёт камеру из БД и создаёт клиент Majestic для неё.
func (s *CameraSettingsService) clientFor(ctx context.Context, id uuid.UUID) (*domain.Camera, *MajesticClient, error) {
	cam, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if cam.IP == "" {
		return nil, nil, fmt.Errorf("у камеры не задан IP-адрес")
	}

	username, password := credentialsFromSettings(cam.Settings)
	return cam, NewMajesticClient(cam.IP, username, password), nil
}

// explain превращает ошибку API в понятное объяснение.
//
// Оператору важно понимать, что делать: неверный пароль исправляется,
// недоступная камера — это сеть, а отсутствие API означает, что
// управлять настройками на этой прошивке нельзя.
func (s *CameraSettingsService) explain(cam *domain.Camera, err error) error {
	if errors.Is(err, ErrNotMajestic) || errors.Is(err, ErrOpenIPCNoMajestic) {
		return fmt.Errorf("камера не поддерживает управление через API: " +
			"на ней нет Majestic (управление возможно только по SSH)")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("камера %s не ответила за отведённое время", cam.IP)
	}
	return fmt.Errorf("камера %s: %w", cam.IP, err)
}

// parseCameraSettings разбирает конфигурацию Majestic в структуру настроек.
// Все поля заполняются указателями: так вызывающий код видит, какие
// значения действительно пришли с камеры.
func parseCameraSettings(cfg map[string]any) CameraSettings {
	out := CameraSettings{}

	if v := section(cfg, "video0"); v != nil {
		out.MainFPS = intValue(v, "fps")
		out.MainBitrate = intValue(v, "bitrate")
		out.MainSize = stringValue(v, "size")
		out.MainCodec = stringValue(v, "codec")
	}
	if v := section(cfg, "video1"); v != nil {
		out.SubEnabled = boolValue(v, "enabled")
		out.SubFPS = intValue(v, "fps")
		out.SubBitrate = intValue(v, "bitrate")
		out.SubSize = stringValue(v, "size")
	}
	if img := section(cfg, "image"); img != nil {
		out.Mirror = boolValue(img, "mirror")
		out.Flip = boolValue(img, "flip")
		out.Contrast = intValue(img, "contrast")
		out.Hue = intValue(img, "hue")
		out.Saturation = intValue(img, "saturation")
		out.Luminance = intValue(img, "luminance")
	}
	if nm := section(cfg, "nightMode"); nm != nil {
		out.NightMode = &NightMode{
			ColorToGray:    boolValue(nm, "colorToGray"),
			IRCut:          stringValue(nm, "irCut"),
			AutoNightDelay: intValue(nm, "autoNightDelay"),
			AutoDayDelay:   intValue(nm, "autoDayDelay"),
			Backlight:      stringValue(nm, "backlight"),
		}
	}
	if isp := section(cfg, "isp"); isp != nil {
		out.AntiFlicker = stringValue(isp, "antiFlicker")
	}
	if osd := section(cfg, "osd"); osd != nil {
		out.OSDEnabled = boolValue(osd, "enabled")
		out.OSDTemplate = stringValue(osd, "template")
		out.OSDSize = stringValue(osd, "size")
		out.OSDPosX = intValue(osd, "posX")
		out.OSDPosY = intValue(osd, "posY")
		out.OSDBgAlpha = intValue(osd, "bgAlpha")
		out.OSDOutline = boolValue(osd, "outline")
	}

	return out
}

// buildSettingsPatch переводит настройки в формат конфигурации Majestic.
//
// В патч попадают только заполненные поля: nil означает, что настройку
// не трогали. Это защищает от главной ошибки такого рода интеграций —
// когда правка одного параметра затирает все остальные значениями
// по умолчанию.
func buildSettingsPatch(s CameraSettings) map[string]any {
	patch := map[string]any{}

	video0 := map[string]any{}
	setIfNotNil(video0, "fps", s.MainFPS)
	setIfNotNil(video0, "bitrate", s.MainBitrate)
	setIfNotNil(video0, "size", s.MainSize)
	setIfNotNil(video0, "codec", s.MainCodec)
	if len(video0) > 0 {
		patch["video0"] = video0
	}

	video1 := map[string]any{}
	setIfNotNil(video1, "fps", s.SubFPS)
	setIfNotNil(video1, "bitrate", s.SubBitrate)
	setIfNotNil(video1, "size", s.SubSize)
	setIfNotNil(video1, "enabled", s.SubEnabled)
	if len(video1) > 0 {
		patch["video1"] = video1
	}

	image := map[string]any{}
	setIfNotNil(image, "mirror", s.Mirror)
	setIfNotNil(image, "flip", s.Flip)
	setIfNotNil(image, "contrast", s.Contrast)
	setIfNotNil(image, "hue", s.Hue)
	setIfNotNil(image, "saturation", s.Saturation)
	setIfNotNil(image, "luminance", s.Luminance)
	if len(image) > 0 {
		patch["image"] = image
	}

	if nm := s.NightMode; nm != nil {
		night := map[string]any{}
		setIfNotNil(night, "colorToGray", nm.ColorToGray)
		setIfNotNil(night, "irCut", nm.IRCut)
		setIfNotNil(night, "autoNightDelay", nm.AutoNightDelay)
		setIfNotNil(night, "autoDayDelay", nm.AutoDayDelay)
		setIfNotNil(night, "backlight", nm.Backlight)
		if len(night) > 0 {
			patch["nightMode"] = night
		}
	}

	isp := map[string]any{}
	setIfNotNil(isp, "antiFlicker", s.AntiFlicker)
	if len(isp) > 0 {
		patch["isp"] = isp
	}

	osd := map[string]any{}
	setIfNotNil(osd, "enabled", s.OSDEnabled)
	setIfNotNil(osd, "template", s.OSDTemplate)
	setIfNotNil(osd, "size", s.OSDSize)
	setIfNotNil(osd, "posX", s.OSDPosX)
	setIfNotNil(osd, "posY", s.OSDPosY)
	setIfNotNil(osd, "bgAlpha", s.OSDBgAlpha)
	setIfNotNil(osd, "outline", s.OSDOutline)
	if len(osd) > 0 {
		patch["osd"] = osd
	}

	return patch
}

// setIfNotNil кладёт значение в патч, только если его задали.
func setIfNotNil[V any](m map[string]any, key string, value *V) {
	if value != nil {
		m[key] = *value
	}
}

// validateCameraSettings проверяет значения до отправки на камеру.
//
// Камера слабая: неверный битрейт или fps роняет поток, а восстановить
// его без доступа к устройству сложно. Поэтому диапазоны проверяем здесь,
// до записи. Проверяются только заполненные поля: nil — настройку не трогали.
func validateCameraSettings(s CameraSettings) error {
	if err := checkRange("fps основного потока", s.MainFPS, 1, 60); err != nil {
		return err
	}
	if err := checkRange("fps дополнительного потока", s.SubFPS, 1, 60); err != nil {
		return err
	}
	if err := checkRange("битрейт основного потока", s.MainBitrate, 64, 20000); err != nil {
		return err
	}
	if err := checkRange("битрейт дополнительного потока", s.SubBitrate, 64, 8000); err != nil {
		return err
	}
	for _, item := range []struct {
		name  string
		value *int
	}{
		{"contrast", s.Contrast}, {"hue", s.Hue},
		{"saturation", s.Saturation}, {"luminance", s.Luminance},
	} {
		if err := checkRange(item.name, item.value, 0, 100); err != nil {
			return err
		}
	}
	if err := checkRange("прозрачность фона OSD", s.OSDBgAlpha, 0, 100); err != nil {
		return err
	}
	if err := checkRange("координата OSD по X", s.OSDPosX, 0, 4096); err != nil {
		return err
	}
	if err := checkRange("координата OSD по Y", s.OSDPosY, 0, 4096); err != nil {
		return err
	}
	if s.OSDTemplate != nil && len(*s.OSDTemplate) > 128 {
		return fmt.Errorf("шаблон OSD длиннее 128 символов")
	}
	if s.MainSize != nil && *s.MainSize != "" && !validSize(*s.MainSize) {
		return fmt.Errorf("неверный размер кадра: %s (ожидается вида 1920x1080)", *s.MainSize)
	}
	if s.SubSize != nil && *s.SubSize != "" && !validSize(*s.SubSize) {
		return fmt.Errorf("неверный размер кадра: %s (ожидается вида 704x576)", *s.SubSize)
	}
	if s.MainCodec != nil && !validChoice(*s.MainCodec, "h264", "h265") {
		return fmt.Errorf("неизвестный кодек: %s", *s.MainCodec)
	}
	if s.AntiFlicker != nil && !validChoice(*s.AntiFlicker, "disabled", "50hz", "60hz") {
		return fmt.Errorf("неизвестный режим антимерцания: %s", *s.AntiFlicker)
	}
	if nm := s.NightMode; nm != nil {
		if nm.IRCut != nil && !validChoice(*nm.IRCut, "auto", "on", "off") {
			return fmt.Errorf("неизвестный режим ИК-фильтра: %s", *nm.IRCut)
		}
		if err := checkRange("задержка перехода в ночной режим", nm.AutoNightDelay, 0, 3600); err != nil {
			return err
		}
		if err := checkRange("задержка перехода в дневной режим", nm.AutoDayDelay, 0, 3600); err != nil {
			return err
		}
	}
	return nil
}

// checkRange проверяет, что заполненное значение попадает в диапазон.
func checkRange(name string, value *int, min, max int) error {
	if value == nil {
		return nil
	}
	if *value < min || *value > max {
		return fmt.Errorf("%s должен быть от %d до %d, получено %d", name, min, max, *value)
	}
	return nil
}

// validSize проверяет формат размера кадра: ШИРИНАxВЫСОТА.
func validSize(size string) bool {
	parts := strings.SplitN(strings.ToLower(size), "x", 2)
	if len(parts) != 2 {
		return false
	}
	for _, p := range parts {
		n, err := strconv.Atoi(strings.TrimSpace(p))
		if err != nil || n <= 0 || n > 8192 {
			return false
		}
	}
	return true
}

func validChoice(value string, allowed ...string) bool {
	for _, a := range allowed {
		if value == a {
			return true
		}
	}
	return false
}

// section возвращает вложенный объект конфигурации.
func section(cfg map[string]any, key string) map[string]any {
	raw, ok := cfg[key]
	if !ok {
		return nil
	}
	m, _ := raw.(map[string]any)
	return m
}

func intValue(m map[string]any, key string) *int {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	switch v := raw.(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		n := v
		return &n
	case string:
		// Часть полей прошивка отдаёт строками ("0", "16").
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return nil
		}
		return &n
	}
	return nil
}

func stringValue(m map[string]any, key string) *string {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	s, ok := raw.(string)
	if !ok {
		return nil
	}
	return &s
}

func boolValue(m map[string]any, key string) *bool {
	raw, ok := m[key]
	if !ok {
		return nil
	}
	b, ok := raw.(bool)
	if !ok {
		return nil
	}
	return &b
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

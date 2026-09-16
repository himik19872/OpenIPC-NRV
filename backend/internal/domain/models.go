package domain

import (
	"time"

	"github.com/google/uuid"
)

// Camera — модель камеры
type Camera struct {
	ID         uuid.UUID      `json:"id"`
	Name       string         `json:"name"`
	RTSPUrl    string         `json:"rtsp_url"`
	MainStream string         `json:"main_stream,omitempty"` // rtsp://.../stream=0
	SubStream  string         `json:"sub_stream,omitempty"`  // rtsp://.../stream=1
	IP         string         `json:"ip,omitempty"`          // 192.168.1.75
	MAC        string         `json:"mac,omitempty"`
	Firmware   string         `json:"firmware,omitempty"`
	SiteID     *uuid.UUID     `json:"site_id,omitempty"`
	WGIP       string         `json:"wg_ip,omitempty"`
	Status     string         `json:"status"` // online, offline, recording
	PTZ        bool           `json:"ptz"`    // поддерживает ли камера поворот (ONVIF PTZ)
	HWInfo     map[string]any `json:"hw_info,omitempty"`
	Settings   map[string]any `json:"settings,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

// Site — объект размещения камер
type Site struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Address   string    `json:"address,omitempty"`
	WGPubKey  string    `json:"wg_pubkey,omitempty"`
	WGIP      string    `json:"wg_ip,omitempty"`
	Timezone  string    `json:"timezone"`
	CreatedAt time.Time `json:"created_at"`
}

// DetectionEvent — событие AI-детекции
type DetectionEvent struct {
	ID            uuid.UUID          `json:"id"`
	CameraID      uuid.UUID          `json:"camera_id"`
	CameraName    string             `json:"camera_name,omitempty"`
	Timestamp     time.Time          `json:"timestamp"`
	ObjectClass   string             `json:"object_class"` // person, car, truck, dog...
	Confidence    float64            `json:"confidence"`
	BBox          map[string]float64 `json:"bbox,omitempty"` // x, y, w, h
	TrackID       *int               `json:"track_id,omitempty"`
	SnapshotPath  string             `json:"snapshot_path,omitempty"`
	ThumbnailPath string             `json:"thumbnail_path,omitempty"`
	Metadata      map[string]any     `json:"metadata,omitempty"`
}

// ACSEvent — событие СКУД
type ACSEvent struct {
	ID           uuid.UUID      `json:"id"`
	ControllerID uuid.UUID      `json:"controller_id"`
	DoorID       string         `json:"door_id"`
	EventType    string         `json:"event_type"` // access_granted, access_denied, door_forced
	CardNumber   string         `json:"card_number,omitempty"`
	UserID       *uuid.UUID     `json:"user_id,omitempty"`
	Timestamp    time.Time      `json:"timestamp"`
	CameraID     *uuid.UUID     `json:"camera_id,omitempty"`
	SnapshotPath string         `json:"snapshot_path,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
}

// ACSController — контроллер СКУД
type ACSController struct {
	ID          uuid.UUID      `json:"id"`
	Name        string         `json:"name"`
	Vendor      string         `json:"vendor"` // hikvision, dahua, promwad
	IP          string         `json:"ip"`
	Port        int            `json:"port"`
	Credentials map[string]any `json:"-"`
	SiteID      *uuid.UUID     `json:"site_id,omitempty"`
	Status      string         `json:"status"` // online, offline
	Config      map[string]any `json:"config,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// Recording — запись видео
type Recording struct {
	ID             uuid.UUID      `json:"id"`
	CameraID       uuid.UUID      `json:"camera_id"`
	StartTime      time.Time      `json:"start_time"`
	EndTime        time.Time      `json:"end_time"`
	Duration       float64        `json:"duration_sec"`
	FilePath       string         `json:"file_path"`
	FileSize       int64          `json:"file_size"`
	Resolution     string         `json:"resolution,omitempty"`
	Codec          string         `json:"codec,omitempty"`
	EventTriggered bool           `json:"event_triggered"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

// User — пользователь системы
type User struct {
	ID           uuid.UUID      `json:"id"`
	Username     string         `json:"username"`
	PasswordHash string         `json:"-"`
	Role         string         `json:"role"` // admin, operator, viewer
	Permissions  map[string]any `json:"permissions,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
}

// --- Входные DTO ---

type CreateCameraRequest struct {
	Name       string `json:"name" validate:"required,min=1,max=255"`
	RTSPUrl    string `json:"rtsp_url"`
	MainStream string `json:"main_stream,omitempty"`
	SubStream  string `json:"sub_stream,omitempty"`
	IP         string `json:"ip,omitempty"`
	MAC        string `json:"mac,omitempty"`
	Firmware   string `json:"firmware,omitempty"`
	SiteID     string `json:"site_id,omitempty"`
	WGIP       string `json:"wg_ip,omitempty"`
	Username   string `json:"username,omitempty"`
	Password   string `json:"password,omitempty"`
	PTZ        bool   `json:"ptz,omitempty"`
}

type UpdateCameraRequest struct {
	Name       *string `json:"name,omitempty"`
	RTSPUrl    *string `json:"rtsp_url,omitempty"`
	MainStream *string `json:"main_stream,omitempty"`
	SubStream  *string `json:"sub_stream,omitempty"`
	IP         *string `json:"ip,omitempty"`
	MAC        *string `json:"mac,omitempty"`
	Firmware   *string `json:"firmware,omitempty"`
	Status     *string `json:"status,omitempty"`
	Username   *string `json:"username,omitempty"`
	Password   *string `json:"password,omitempty"`
	PTZ        *bool   `json:"ptz,omitempty"`
}

// ScanRequest — запрос на сканирование подсети для поиска OpenIPC-камер
type ScanRequest struct {
	Subnet   string `json:"subnet" validate:"required"` // "192.168.1.0/24"
	Username string `json:"username"`                   // для Majestic API
	Password string `json:"password"`
}

// DiscoveredCamera — камера, найденная при сканировании
type DiscoveredCamera struct {
	IP         string `json:"ip"`
	MAC        string `json:"mac,omitempty"`
	Firmware   string `json:"firmware,omitempty"`
	Model      string `json:"model,omitempty"`
	Vendor     string `json:"vendor,omitempty"` // openipc, hikvision, dahua, onvif, generic
	MainStream string `json:"main_stream"`
	SubStream  string `json:"sub_stream"`
	Snapshot   string `json:"snapshot,omitempty"`
	Username   string `json:"username,omitempty"` // учётные данные, которые подошли
	Password   string `json:"password,omitempty"`
	Online     bool   `json:"online"`
}

// ScanResult — результат сканирования подсети
type ScanResult struct {
	Subnet  string             `json:"subnet"`
	Total   int                `json:"total"` // всего просканировано IP
	Found   int                `json:"found"` // найдено камер
	Cameras []DiscoveredCamera `json:"cameras"`
}

type CreateACSControllerRequest struct {
	Name     string `json:"name" validate:"required"`
	Vendor   string `json:"vendor" validate:"required,oneof=hikvision dahua promwad"`
	IP       string `json:"ip" validate:"required,ip"`
	Port     int    `json:"port" validate:"min=1,max=65535"`
	Login    string `json:"login"`
	Password string `json:"password"`
	SiteID   string `json:"site_id,omitempty"`
}

type LoginRequest struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

type LoginResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	User      User   `json:"user"`
}

type OpenDoorRequest struct {
	DoorID string `json:"door_id" validate:"required"`
}

// PaginatedEventResponse — пагинированный список событий
type PaginatedEvents struct {
	Events   []DetectionEvent `json:"events"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

type PaginatedRecordings struct {
	Recordings []Recording `json:"recordings"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"page_size"`
}

// Stats — статистика системы
type Stats struct {
	TotalCameras   int     `json:"total_cameras"`
	OnlineCameras  int     `json:"online_cameras"`
	TotalEvents24h int64   `json:"total_events_24h"`
	DiskUsedGB     float64 `json:"disk_used_gb"`
	DiskTotalGB    float64 `json:"disk_total_gb"`
	ACSOnline      int     `json:"acs_online"`
	ACSTotal       int     `json:"acs_total"`
}

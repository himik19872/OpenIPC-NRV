package tunnel

import (
	"fmt"
	"net"

	"github.com/rs/zerolog/log"
)

// WireGuardManager — управление WireGuard-пирами через netlink
type WireGuardManager struct {
	iface string
}

func NewWireGuardManager(iface string) (*WireGuardManager, error) {
	// Проверяем существование интерфейса
	_, err := net.InterfaceByName(iface)
	if err != nil {
		return nil, fmt.Errorf("wireguard interface %s not found: %w", iface, err)
	}

	log.Info().Str("iface", iface).Msg("wireguard manager initialized")
	return &WireGuardManager{iface: iface}, nil
}

// PeerStatus — статус WireGuard-пира
type PeerStatus struct {
	PublicKey  string `json:"public_key"`
	Endpoint   string `json:"endpoint"`
	AllowedIPs string `json:"allowed_ips"`
	LastSeen   string `json:"last_seen,omitempty"`
	Online     bool   `json:"online"`
}

// AddPeer добавляет новый пир
func (w *WireGuardManager) AddPeer(pubkey string, ip string) error {
	log.Info().Str("pubkey", pubkey).Str("ip", ip).Msg("adding wireguard peer")
	// Реальная имплементация через netlink:
	// wgtypes.ParseKey(pubkey) → client.ConfigureDevice(iface, config)
	// Пока заглушка
	return nil
}

// RemovePeer удаляет пир
func (w *WireGuardManager) RemovePeer(pubkey string) error {
	log.Info().Str("pubkey", pubkey).Msg("removing wireguard peer")
	return nil
}

// GetPeerStatus возвращает статус пира
func (w *WireGuardManager) GetPeerStatus(pubkey string) (*PeerStatus, error) {
	// Чтение через netlink wgtypes.Client
	return &PeerStatus{
		PublicKey: pubkey,
		Online:    false,
	}, nil
}

// GenerateConfig генерирует конфиг для edge-агента
func (w *WireGuardManager) GenerateConfig(siteID string, serverPubIP string) string {
	// Заглушка — в реальности генерирует ключи и возвращает конфиг
	return fmt.Sprintf(`[Interface]
Address = 10.99.0.0/24
PrivateKey = <generated>

[Peer]
PublicKey = <server_key>
Endpoint = %s:51820
AllowedIPs = 10.99.0.0/24
PersistentKeepalive = 25
`, serverPubIP)
}

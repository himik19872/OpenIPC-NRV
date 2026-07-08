#!/bin/bash
# NRV — скрипт установки systemd-сервисов для автозапуска
# Запуск: sudo bash scripts/install-services.sh

set -e

SERVICES_DIR="/etc/systemd/system"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

echo "=== NRV: Установка systemd-сервисов ==="

for svc in nrv-backend nrv-frontend nrv-go2rtc; do
    echo "Устанавливаю $svc..."
    cp "$SCRIPT_DIR/$svc.service" "$SERVICES_DIR/$svc.service"
    systemctl daemon-reload
    systemctl enable "$svc"
    systemctl restart "$svc"
    systemctl status "$svc" --no-pager -l | head -3
    echo ""
done

echo "=== Готово! ==="
echo "  Бэкенд:    http://$(hostname -I | awk '{print $1}'):8000/api/docs"
echo "  Фронтенд:  http://$(hostname -I | awk '{print $1}'):5173/"
echo "  go2rtc:    http://$(hostname -I | awk '{print $1}'):1984/api/streams"
echo ""
echo "Управление:"
echo "  sudo systemctl start|stop|restart nrv-backend"
echo "  sudo systemctl start|stop|restart nrv-frontend"
echo "  sudo systemctl start|stop|restart nrv-go2rtc"
echo "  sudo journalctl -u nrv-backend -f   # логи"

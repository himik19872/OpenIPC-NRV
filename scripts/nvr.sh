#!/usr/bin/env bash
#
# Управление сервером видеонаблюдения.
#
# Использование:
#   ./scripts/nvr.sh start      — запустить все сервисы
#   ./scripts/nvr.sh stop       — остановить
#   ./scripts/nvr.sh restart    — перезапустить
#   ./scripts/nvr.sh status     — показать состояние и адреса
#   ./scripts/nvr.sh logs       — логи всех сервисов
#   ./scripts/nvr.sh update     — пересобрать образы и перезапустить
#   ./scripts/nvr.sh backup     — дамп БД в ./backups
#
# Скрипт рассчитан на запуск из корня репозитория.

set -euo pipefail

cd "$(dirname "$0")/.."

COMPOSE="docker compose"
BACKUP_DIR="./backups"

# Цвета для читаемого вывода (если терминал их поддерживает).
if [ -t 1 ]; then
  GREEN='\033[0;32m'; YELLOW='\033[0;33m'; RED='\033[0;31m'; BOLD='\033[1m'; NC='\033[0m'
else
  GREEN=''; YELLOW=''; RED=''; BOLD=''; NC=''
fi

log()  { printf "${GREEN}==>${NC} %s\n" "$1"; }
warn() { printf "${YELLOW}!!!${NC} %s\n" "$1"; }
fail() { printf "${RED}xxx${NC} %s\n" "$1" >&2; exit 1; }

require_docker() {
  command -v docker >/dev/null 2>&1 || fail "Docker не установлен"
  docker compose version >/dev/null 2>&1 || fail "Docker Compose v2 не найден"
}

require_env() {
  if [ ! -f .env ]; then
    warn "Файл .env не найден, копирую из .env.example"
    cp .env.example .env
    warn "Проверьте .env: обязательно задайте JWT_SECRET и DB_PASSWORD"
  fi
}

cmd_start() {
  require_docker
  require_env
  log "Запускаю сервисы..."
  $COMPOSE up -d
  log "Готово. Состояние:"
  cmd_status
}

cmd_stop() {
  require_docker
  log "Останавливаю сервисы..."
  $COMPOSE down
  log "Остановлено"
}

cmd_restart() {
  require_docker
  log "Перезапускаю сервисы..."
  $COMPOSE restart
  cmd_status
}

cmd_update() {
  require_docker
  require_env
  log "Пересобираю образы и перезапускаю..."
  $COMPOSE up -d --build
  log "Обновление завершено"
}

cmd_logs() {
  require_docker
  $COMPOSE logs -f --tail=100 "${@:-}"
}

cmd_backup() {
  require_docker
  mkdir -p "$BACKUP_DIR"
  local stamp file
  stamp=$(date +%Y%m%d_%H%M%S)
  file="${BACKUP_DIR}/nvr_${stamp}.sql"

  log "Создаю дамп базы данных в ${file}..."
  if $COMPOSE exec -T postgres pg_dump -U nvr nvr > "$file"; then
    local size
    size=$(du -h "$file" | cut -f1)
    log "Дамп готов: ${file} (${size})"
  else
    rm -f "$file"
    fail "Не удалось создать дамп. Проверьте, что контейнер postgres запущен"
  fi
}

cmd_status() {
  require_docker
  echo ""
  $COMPOSE ps
  echo ""

  # Достаём адрес сервера, чтобы показать готовые ссылки.
  local ip
  ip=$(hostname -I 2>/dev/null | awk '{print $1}')
  [ -z "$ip" ] && ip="<IP сервера>"

  printf "${BOLD}Адреса для доступа:${NC}\n"
  printf "  Веб-интерфейс     http://%s:3001\n" "$ip"
  printf "  API               http://%s:8080/api/v1\n" "$ip"
  printf "  Документация API  http://%s:8080/api/v1/docs\n" "$ip"
  printf "  MinIO Console     http://%s:9001\n" "$ip"
  printf "  MediaMTX API      http://%s:9997\n" "$ip"
  echo ""
  printf "${BOLD}Потоки для внешних систем:${NC}\n"
  printf "  rtsp://ЛОГИН:ПАРОЛЬ@%s:9784/cameras/{НОМЕР-1}/streaming/main\n" "$ip"
  printf "  Список каналов и ссылки — на странице «Внешний доступ»\n"
  echo ""
  printf "  Логин по умолчанию: admin / admin123 ${YELLOW}(смените после первого входа!)${NC}\n"
  echo ""
}

cmd_help() {
  sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
}

case "${1:-help}" in
  start)   cmd_start ;;
  stop)    cmd_stop ;;
  restart) cmd_restart ;;
  update)  cmd_update ;;
  status)  cmd_status ;;
  logs)    shift; cmd_logs "$@" ;;
  backup)  cmd_backup ;;
  help|-h|--help) cmd_help ;;
  *) fail "Неизвестная команда: $1. Запустите ./scripts/nvr.sh help" ;;
esac

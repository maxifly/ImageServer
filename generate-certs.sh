#!/bin/bash
set -e

# Загружаем переменные из .env, если существует
if [ -f ".env" ]; then
  export $(grep -v '^#' .env | xargs)
fi

# Определяем корневой каталог данных
DATA_ROOT="${IMAGE_SERVER_DATA_ROOT:-/opt/image-server-data}"
CERTS_DIR="$DATA_ROOT/certs"

echo "Путь к данным: $DATA_ROOT"
mkdir -p "$CERTS_DIR"

if [[ -f "$CERTS_DIR/cert.pem" && -f "$CERTS_DIR/key.pem" ]]; then
  echo "Сертификаты уже существуют в $CERTS_DIR"
  echo "Если хотите пересоздать — удалите папку certs и запустите скрипт снова."
  exit 0
fi

read -p "Введите локальный IP-адрес сервера (например, 192.168.1.100), или Enter для пропуска: " LOCAL_IP

SAN="IP:127.0.0.1,DNS:localhost"
if [[ -n "$LOCAL_IP" && "$LOCAL_IP" =~ ^[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}$ ]]; then
  SAN="$SAN,IP:$LOCAL_IP"
  echo "Сертификат будет валиден для: localhost, 127.0.0.1, $LOCAL_IP"
else
  echo "Сертификат будет валиден только для localhost и 127.0.0.1"
fi

openssl req -x509 -newkey rsa:4096 -sha256 -days 3650 -nodes \
  -keyout "$CERTS_DIR/key.pem" -out "$CERTS_DIR/cert.pem" \
  -subj "/CN=ImageServer" \
  -addext "subjectAltName=$SAN"

echo ""
echo "Сертификаты созданы в $CERTS_DIR"
echo "Теперь запустите: docker compose up -d"
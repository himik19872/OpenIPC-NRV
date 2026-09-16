FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:3.20

# openssh-client + sshpass — для управления камерами OpenIPC (restart Majestic, reboot).
# ffmpeg — для получения кадра из HLS-потока, когда камера не отдаёт JPEG напрямую
# (например, OpenIPC при занятых аппаратных скейлерах возвращает 503 на /image.jpg).
RUN apk --no-cache add ca-certificates tzdata openssh-client sshpass ffmpeg

WORKDIR /app

COPY --from=builder /server .
COPY migrations/ ./migrations/

EXPOSE 8080

ENTRYPOINT ["./server"]
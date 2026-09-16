package minio

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/rs/zerolog/log"
)

// VideoRepo — работа с MinIO / S3-хранилищем видеоархива.
//
// Записи хранятся как объекты по ключу вида `{cameraID}/{date}/{time}.mp4`.
// Файлы отдаются клиенту через временные presigned-ссылки, чтобы не гонять
// видео через бэкенд и не раскрывать ключи доступа.
type VideoRepo struct {
	client *minio.Client
	// signClient настроен на внешний адрес MinIO и используется только
	// для генерации presigned-ссылок (подпись зависит от Host).
	signClient     *minio.Client
	bucket         string
	publicEndpoint string
	publicUseSSL   bool
}

// NewVideoRepo подключается к MinIO и создаёт бакет, если его нет.
//
// endpoint — внутренний адрес для обмена данными (например, "minio:9000").
// publicEndpoint — адрес для presigned-ссылок, которые получит браузер;
// если пуст, используется endpoint.
func NewVideoRepo(endpoint, publicEndpoint, accessKey, secretKey, bucket string, useSSL bool) (*VideoRepo, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("minio endpoint is empty")
	}
	if bucket == "" {
		bucket = "nvr-recordings"
	}

	// Для подписи ссылок нужен клиент, настроенный на внешний адрес:
	// подпись зависит от Host, поэтому «подписать» внутренним нельзя —
	// браузер получит ссылку на недоступный ему адрес или неверную подпись.
	signEndpoint := publicEndpoint
	if signEndpoint == "" {
		signEndpoint = endpoint
	}

	creds := credentials.NewStaticV4(accessKey, secretKey, "")
	client, err := minio.New(endpoint, &minio.Options{Creds: creds, Secure: useSSL})
	if err != nil {
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	signClient := client
	if signEndpoint != endpoint {
		signClient, err = minio.New(signEndpoint, &minio.Options{Creds: creds, Secure: useSSL})
		if err != nil {
			return nil, fmt.Errorf("create minio signing client: %w", err)
		}
	}

	repo := &VideoRepo{
		client:         client,
		bucket:         bucket,
		publicEndpoint: signEndpoint,
		publicUseSSL:   useSSL,
	}

	// Создание бакета идемпотентно: если он уже есть, ничего не произойдёт.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	exists, err := client.BucketExists(ctx, bucket)
	if err != nil {
		return nil, fmt.Errorf("check bucket: %w", err)
	}
	if !exists {
		if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, fmt.Errorf("create bucket %q: %w", bucket, err)
		}
		log.Info().Str("bucket", bucket).Msg("minio bucket created")
	}

	repo.signClient = signClient

	log.Info().Str("endpoint", endpoint).Str("public", signEndpoint).Str("bucket", bucket).
		Msg("connected to minio")
	return repo, nil
}

// Bucket возвращает имя используемого бакета.
func (r *VideoRepo) Bucket() string {
	return r.bucket
}

// BuildKey формирует ключ объекта для записи камеры.
func BuildKey(cameraID string, start time.Time, ext string) string {
	if ext == "" {
		ext = "mp4"
	}
	return fmt.Sprintf("%s/%s/%s.%s",
		cameraID,
		start.UTC().Format("2006-01-02"),
		start.UTC().Format("150405"),
		strings.TrimPrefix(ext, "."),
	)
}

// Upload загружает файл записи в хранилище.
func (r *VideoRepo) Upload(ctx context.Context, key string, reader io.Reader, size int64, contentType string) error {
	if contentType == "" {
		contentType = "video/mp4"
	}
	_, err := r.client.PutObject(ctx, r.bucket, key, reader, size,
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return fmt.Errorf("upload %q: %w", key, err)
	}
	return nil
}

// PresignedURL возвращает временную ссылку на объект.
// По ней клиент скачает видео напрямую из MinIO.
// Подпись делается клиентом, настроенным на внешний адрес.
func (r *VideoRepo) PresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if expiry <= 0 {
		expiry = time.Hour
	}
	client := r.signClient
	if client == nil {
		client = r.client
	}
	u, err := client.PresignedGetObject(ctx, r.bucket, key, expiry, nil)
	if err != nil {
		return "", fmt.Errorf("presign %q: %w", key, err)
	}
	return u.String(), nil
}

// Delete удаляет объект записи.
func (r *VideoRepo) Delete(ctx context.Context, key string) error {
	err := r.client.RemoveObject(ctx, r.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("delete %q: %w", key, err)
	}
	return nil
}

// Exists проверяет наличие объекта.
func (r *VideoRepo) Exists(ctx context.Context, key string) (bool, error) {
	_, err := r.client.StatObject(ctx, r.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		resp := minio.ToErrorResponse(err)
		if resp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

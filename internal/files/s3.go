package files

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/pkg/errors"
)

type S3FileManager struct {
	client *minio.Client
	bucket string
}

func NewS3FileManager(endpoint, accessKeyID, secretAccessKey, bucket string, useSSL bool) (*S3FileManager, error) {
	minioClient, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKeyID, secretAccessKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create Minio client")
	}

	return &S3FileManager{
		client: minioClient,
		bucket: bucket,
	}, nil
}

func (fm *S3FileManager) Read(ctx context.Context, path string) ([]byte, error) {
	object, err := fm.client.GetObject(ctx, fm.bucket, path, minio.GetObjectOptions{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get object")
	}
	defer func(object *minio.Object) {
		err := object.Close()
		if err != nil {
			slog.Warn(fmt.Sprintf("failed to close object: %s", err))
		}
	}(object)

	buf := new(bytes.Buffer)
	_, err = io.Copy(buf, object)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read object")
	}

	return buf.Bytes(), nil
}

func (fm *S3FileManager) Write(ctx context.Context, path string, data []byte) error {
	reader := bytes.NewReader(data)
	_, err := fm.client.PutObject(ctx, fm.bucket, path, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: "application/octet-stream",
	})
	if err != nil {
		return errors.Wrap(err, "failed to put object")
	}

	return nil
}

func (fm *S3FileManager) Delete(ctx context.Context, path string) error {
	err := fm.client.RemoveObject(ctx, fm.bucket, path, minio.RemoveObjectOptions{})
	if err != nil {
		return errors.Wrap(err, "failed to remove object")
	}

	return nil
}

func (fm *S3FileManager) Exists(ctx context.Context, path string) bool {
	_, err := fm.client.StatObject(ctx, fm.bucket, path, minio.StatObjectOptions{})

	return err == nil
}

func (fm *S3FileManager) List(ctx context.Context, dir string) ([]string, error) {
	prefix := dir
	if !strings.HasSuffix(prefix, "/") && prefix != "" {
		prefix += "/"
	}

	objectCh := fm.client.ListObjects(ctx, fm.bucket, minio.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: false,
	})

	var files []string
	for object := range objectCh {
		if object.Err != nil {
			return nil, errors.Wrap(object.Err, "failed to list objects")
		}

		// Remove the prefix to get relative path
		name := strings.TrimPrefix(object.Key, prefix)
		if name != "" {
			files = append(files, name)
		}
	}

	return files, nil
}

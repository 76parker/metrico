// Package filestorage сохраняет и восстанавливает проверяемые JSON-снимки метрик.
package filestorage

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"

	"github.com/76Parker/metrico/internal/adapters/filestorage/snapshotschema"
	"github.com/76Parker/metrico/internal/domain/metrics"
	goccyjson "github.com/goccy/go-json"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Storage сохраняет и восстанавливает JSON-снимки метрик с проверкой схемы.
// Содержит мьютекс и не должен копироваться после использования.
type Storage struct {
	fileStoragePath string
	schema          *jsonschema.Schema
	mu              sync.Mutex
}

// NewStorage подготавливает файловое хранилище и компилирует встроенную JSON Schema.
// Файл снимка открывается только при Save или Restore.
func NewStorage(fileStoragePath string) (*Storage, error) {

	compiler := jsonschema.NewCompiler()
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(snapshotschema.MetricsSnapshotV1SchemaJSON))
	if err != nil {
		return nil, fmt.Errorf("parse embedded snapshot schema: %w", err)
	}

	const schemaURL = "metrics-snapshot-v1.schema.json"
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		return nil, fmt.Errorf("register embedded snapshot schema: %w", err)
	}

	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		return nil, fmt.Errorf("compile embedded snapshot schema: %w", err)
	}

	return &Storage{
		fileStoragePath: fileStoragePath,
		schema:          schema,
		mu:              sync.Mutex{},
	}, nil
}

// Save проверяет снимок по JSON Schema и записывает его через временный файл
// с последующим переименованием. Каталог назначения должен существовать.
func (s *Storage) Save(ctx context.Context, metricsSlice []metrics.Metrics) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	metricSnapshot := make(snapshotschema.MetricsSnapshotV1, 0, len(metricsSlice))
	for _, metric := range metricsSlice {
		m := snapshotschema.Metric{
			Id:    metric.ID,
			Type:  snapshotschema.MetricType(metric.Type),
			Delta: metric.Delta,
			Value: metric.Value,
		}
		metricSnapshot = append(metricSnapshot, m)
	}
	data, err := goccyjson.MarshalIndent(metricSnapshot, "", "  ")
	if err != nil {
		return err
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return err
	}
	if err := s.schema.Validate(doc); err != nil {
		return err
	}
	if err := s.createNewSnapshot(data); err != nil {
		return err
	}
	return nil
}

// Restore читает снимок и проверяет его по JSON Schema.
// Для отсутствующего или пустого файла возвращает пустой набор метрик.
func (s *Storage) Restore(ctx context.Context) ([]metrics.Metrics, error) {
	rawFile, err := os.ReadFile(s.fileStoragePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []metrics.Metrics{}, nil
		}
		return nil, err
	}
	if len(bytes.TrimSpace(rawFile)) == 0 {
		return []metrics.Metrics{}, nil
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(rawFile))
	if err != nil {
		return nil, err
	}
	if err := s.schema.Validate(doc); err != nil {
		return nil, err
	}
	var snapshotFromFile snapshotschema.MetricsSnapshotV1
	if err := goccyjson.Unmarshal(rawFile, &snapshotFromFile); err != nil {
		return nil, err
	}
	snapshotLen := len(snapshotFromFile)

	metricsSlice := make([]metrics.Metrics, 0, snapshotLen)
	for _, metric := range snapshotFromFile {
		m := metrics.Metrics{
			ID:    metric.Id,
			Type:  metrics.MetricType(metric.Type),
			Delta: metric.Delta,
			Value: metric.Value,
		}
		metricsSlice = append(metricsSlice, m)
	}
	return metricsSlice, nil
}

func (s *Storage) createNewSnapshot(snapshotData []byte) error {
	dir := filepath.Dir(s.fileStoragePath)
	snapshotID := uuid.New().String()
	newSnapshotName := dir + "/snapshot_" + snapshotID + ".json"
	snapshotFile, err := os.Create(newSnapshotName)
	if err != nil {
		return err
	}
	if _, err := snapshotFile.Write(snapshotData); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	if err := snapshotFile.Sync(); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	if err := snapshotFile.Close(); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	if err := os.Rename(newSnapshotName, s.fileStoragePath); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	dirFile, err := os.Open(dir)
	if err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	if err := dirFile.Sync(); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	if err := dirFile.Close(); err != nil {
		deleteSnapshot(newSnapshotName)
		return err
	}
	return nil
}

func deleteSnapshot(snapshotPath string) {
	os.Remove(snapshotPath)
}

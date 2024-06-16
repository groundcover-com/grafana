package historian

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana/pkg/services/ngalert/metrics"
	"github.com/unknwon/log"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

var _ remoteLokiClient = (*otelLokiClient)(nil)

type otelLokiClient struct {
	client  plogotlp.GRPCClient
	once    *sync.Once
	cfg     OtelConfig
	metrics *metrics.Historian
}

func newOtelLokiClient(cfg OtelConfig, metrics *metrics.Historian) *otelLokiClient {
	return &otelLokiClient{
		once:    &sync.Once{},
		cfg:     cfg,
		metrics: metrics,
	}
}

func (p *otelLokiClient) ping(context.Context) error {
	return nil
}

func (p *otelLokiClient) rangeQuery(ctx context.Context, logQL string, start, end, limit int64) (queryRes, error) {
	return queryRes{}, fmt.Errorf("unsupported operation")
}

func (p *otelLokiClient) push(ctx context.Context, s []stream) (err error) {
	const (
		timerFailureCode = "500"
		exportMethodName = "OtelExport"
	)
	p.once.Do(func() {
		var conn *grpc.ClientConn
		conn, err = newOtlpGrpcConn(p.cfg)
		if err != nil {
			return
		}
		p.client = plogotlp.NewGRPCClient(conn)
	})

	if err != nil {
		return err
	}

	logs, size, err := p.pushRequestToLogs(s, time.Now())
	if err != nil {
		return err
	}

	exportStart := time.Now()
	p.metrics.WriteDuration.Before(ctx, exportMethodName, exportStart)
	_, err = p.client.Export(ctx, plogotlp.NewExportRequestFromLogs(logs))
	if err != nil {
		return fmt.Errorf("failed to export logs: %w", err)
	}
	p.metrics.WriteDuration.After(ctx, exportMethodName, timerFailureCode, exportStart)
	p.metrics.BytesWritten.Add(float64(size))
	return nil
}

func (p *otelLokiClient) pushRequestToLogs(sreams []stream, defaultTimestamp time.Time) (plog.Logs, int, error) {
	logs := plog.NewLogs()
	if len(sreams) == 0 {
		return logs, 0, nil
	}
	rls := logs.ResourceLogs().AppendEmpty()
	logSlice := rls.ScopeLogs().AppendEmpty().LogRecords()
	totalSize := 0

	var lastErr error
	var errNumber int64
	for _, stream := range sreams {
		// Return early if stream does not contain any entries
		if len(stream.Stream) == 0 {
			continue
		}

		totalSize += calcAttributesSize(stream.Stream)

		for _, entry := range stream.Values {
			lr := logSlice.AppendEmpty()
			convertEntryToLogRecord(entry, stream.Stream, &lr, defaultTimestamp)
			totalSize += len(entry.V)
		}
	}

	if lastErr != nil {
		lastErr = fmt.Errorf("%d entries failed to process, the last error: %w", errNumber, lastErr)
	}

	return logs, totalSize, lastErr
}

func convertEntryToLogRecord(entry sample, streamAttributes map[string]string, lr *plog.LogRecord, defaultTimestamp time.Time) error {
	const timestampAttribute = "timestamp"

	observedTimestamp := pcommon.NewTimestampFromTime(defaultTimestamp)
	lr.SetObservedTimestamp(observedTimestamp)

	var recordAttributes map[string]any
	err := json.Unmarshal([]byte(entry.V), &recordAttributes)
	if err != nil {
		return fmt.Errorf("failed to unmarshal log line: %w", err)
	}

	var timestamp pcommon.Timestamp
	if !entry.T.IsZero() {
		timestamp = pcommon.NewTimestampFromTime(entry.T)
	} else {
		timestamp = observedTimestamp
	}

	lr.SetTimestamp(timestamp)
	recordAttributes[timestampAttribute] = timestamp.AsTime().Format(time.RFC3339Nano)

	for k, v := range streamAttributes {
		recordAttributes[k] = v
	}

	lr.Attributes().FromRaw(recordAttributes)
	return nil
}

func calcAttributesSize(attributes map[string]string) int {
	size := 0
	for k, v := range attributes {
		size += len(k) + len(v)
	}
	return size
}

func newOtlpGrpcConn(cfg OtelConfig) (conn *grpc.ClientConn, err error) {
	const (
		apiKeyHeader                 = "apikey"
		defaultConnectionDialTimeout = 10 * time.Second
	)
	creds := insecure.NewCredentials()
	if cfg.EnableTLS {
		config := &tls.Config{
			InsecureSkipVerify: cfg.TLSSkipVerify,
		}
		creds = credentials.NewTLS(config)
		log.Info("Establishing grpcs connection")
	} else {
		log.Info("Establishing not encrypted grpc connection")
	}

	options := []grpc.DialOption{
		grpc.WithTransportCredentials(creds),
	}

	if cfg.ApiKey != "" {
		options = append(options, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req interface{}, reply interface{},
			cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {

			ctx = metadata.AppendToOutgoingContext(ctx, apiKeyHeader, cfg.ApiKey)
			return invoker(ctx, method, req, reply, cc, opts...)
		}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), defaultConnectionDialTimeout)
	defer cancel()
	return grpc.DialContext(ctx, cfg.Endpoint, options...)
}

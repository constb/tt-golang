package utils

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	HeaderContentType   = "Content-Type"
	HeaderContentLength = "Content-Length"
	HeaderAccept        = "Accept"
	HeaderRequestId     = "X-Request-ID"
	HeaderApiKey        = "X-Api-Key"

	MediaTypeJson     = "application/json"
	MediaTypeProtobuf = "application/protobuf"
)

var (
	ErrContentTypeRequired = errors.New(`content-type header required`)
	ErrMessageNoInterface  = errors.New("message doesn't implement protobuf interface")
)

func OnlyMethod(h http.Handler, allow string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == allow {
			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func RequestID(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get(HeaderRequestId)
		if reqID == "" {
			reqID = GenerateID().Base32()
			r.Header.Set(HeaderRequestId, reqID)
		}

		w.Header().Set(HeaderRequestId, reqID)

		h.ServeHTTP(w, r)
	})
}

func APIKey(h http.Handler, key string) http.Handler {
	if key == "" {
		return h
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqKey := r.Header.Get(HeaderApiKey)
		if reqKey == key {
			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusForbidden)
		}
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *loggingResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func RequestLogger(h http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqLogger := logger.With(zap.String("reqId", r.Header.Get(HeaderRequestId)))
		reqLogger.Info("incoming request",
			zap.Object("req", zapcore.ObjectMarshalerFunc(func(encoder zapcore.ObjectEncoder) error {
				encoder.AddString("method", r.Method)
				encoder.AddString("url", r.RequestURI)
				encoder.AddString("remoteAddress", r.RemoteAddr)
				return nil
			})),
		)
		lw := &loggingResponseWriter{w, http.StatusOK}
		h.ServeHTTP(lw, r.WithContext(context.WithValue(r.Context(), "Logger", reqLogger)))
		reqLogger.Info("request completed", zap.Object("res", zapcore.ObjectMarshalerFunc(func(encoder zapcore.ObjectEncoder) error {
			encoder.AddString("url", r.RequestURI)
			encoder.AddInt("statusCode", lw.statusCode)
			return nil
		})), zap.Float64("responseTime", time.Since(start).Seconds()*1000.0))
	})
}

func GetRequestLogger(r *http.Request) *zap.Logger {
	l, ok := r.Context().Value("Logger").(*zap.Logger)
	if !ok {
		return logger
	}
	return l
}

func UnmarshalInput(r *http.Request, message any) error {
	ct := r.Header.Get(HeaderContentType)
	if ct == "" {
		return ErrContentTypeRequired
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return err
	}
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return ErrMessageNoInterface
	}

	if mt == MediaTypeJson {
		defer r.Body.Close()
		bytes, err := io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if err = protojson.Unmarshal(bytes, protoMessage); err != nil {
			return fmt.Errorf("unmarshal json: %w", err)
		}
	} else if mt == MediaTypeProtobuf {
		defer r.Body.Close()
		bytes, err := io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
		if err = proto.Unmarshal(bytes, protoMessage); err != nil {
			return fmt.Errorf("unmarshal protobuf: %w", err)
		}
	}

	return nil
}

func WriteOutput(r *http.Request, w http.ResponseWriter, logger *zap.Logger, message any) {
	useJSON := true
	mt, _, _ := mime.ParseMediaType(r.Header.Get(HeaderAccept))
	if mt == MediaTypeProtobuf {
		useJSON = false
	} else if mt == "" {
		mt, _, _ := mime.ParseMediaType(r.Header.Get(HeaderContentType))
		if mt == MediaTypeProtobuf {
			useJSON = false
		}
	}

	protoMessage, ok := message.(proto.Message)
	var bytes []byte
	var err error
	if !ok {
		logger.Error("marshal protobuf", zap.Error(ErrMessageNoInterface))
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if useJSON {
		bytes, err = protojson.Marshal(protoMessage)
		if err != nil {
			logger.Error("marshal json", zap.Error(err))
		} else {
			w.Header().Set(HeaderContentType, "application/json; charset=utf-8")
		}
	} else {
		bytes, err = proto.Marshal(protoMessage)
		if err != nil {
			logger.Error("marshal protobuf", zap.Error(err))
		} else {
			w.Header().Set(HeaderContentType, "application/protobuf")
		}
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set(HeaderContentLength, strconv.Itoa(len(bytes)))
	w.WriteHeader(http.StatusOK)
	_, err = w.Write(bytes)
	if err != nil {
		logger.Error("writing response", zap.Error(err))
	}
}

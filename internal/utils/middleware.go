package utils

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"go.uber.org/zap"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	HeaderContentType   = "Content-Type"
	HeaderContentLength = "Content-Length"
	HeaderAccept        = "Accept"
	HeaderRequestId     = "X-Request-ID"

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

func RequestLogger(r *http.Request, logger *zap.Logger) *zap.Logger {
	return logger.With(zap.String("reqId", r.Header.Get(HeaderRequestId)))
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

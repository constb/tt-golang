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

func OnlyMethod(h http.Handler, allow string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == allow {
			h.ServeHTTP(w, r)
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
}

func RequestId(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqId := r.Header.Get(HeaderRequestId)
		if reqId == "" {
			reqId = GenerateID().Base32()
			r.Header.Set(HeaderRequestId, reqId)
		}

		w.Header().Set(HeaderRequestId, reqId)

		h.ServeHTTP(w, r)
	})
}

func RequestLogger(r *http.Request, logger *zap.Logger) *zap.Logger {
	return logger.With(zap.String("reqId", r.Header.Get(HeaderRequestId)))
}

func UnmarshalInput(r *http.Request, message any) error {
	ct := r.Header.Get(HeaderContentType)
	if ct == "" {
		return fmt.Errorf(`content-type header required`)
	}
	mt, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return err
	}
	protoMessage, ok := message.(proto.Message)
	if !ok {
		return errors.New("message doesn't implement protobuf interface")
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
	useJson := true
	mt, _, _ := mime.ParseMediaType(r.Header.Get(HeaderAccept))
	if mt == MediaTypeProtobuf {
		useJson = false
	} else if mt == "" {
		mt, _, _ := mime.ParseMediaType(r.Header.Get(HeaderContentType))
		if mt == MediaTypeProtobuf {
			useJson = false
		}
	}

	protoMessage, ok := message.(proto.Message)
	var bytes []byte
	var err error
	if !ok {
		logger.Error("marshal protobuf", zap.Error(errors.New("message doesn't implement protobuf interface")))
		w.WriteHeader(500)
		return
	}
	if useJson {
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
		w.WriteHeader(500)
		return
	}

	w.Header().Set(HeaderContentLength, strconv.Itoa(len(bytes)))
	w.WriteHeader(200)
	_, err = w.Write(bytes)
	if err != nil {
		logger.Error("writing response", zap.Error(err))
	}
}

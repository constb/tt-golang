package utils

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/constb/tt-golang/internal/proto"
	"github.com/stretchr/testify/assert"
)

func TestOnlyMethod(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		allow      string
		wantCalled bool
		wantStatus int
	}{
		{"HEAD", "HEAD", "POST", false, 405},
		{"GET", "GET", "POST", false, 405},
		{"POST", "POST", "POST", true, 200},
		{"OPTIONS", "OPTIONS", "POST", false, 405},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var called bool
			mw := OnlyMethod(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				called = true
			}), tt.allow)
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, httptest.NewRequest(tt.method, "/", nil))
			response := rr.Result()
			defer response.Body.Close()
			assert.Equalf(t, tt.wantCalled, called, "OnlyMethod: %s/%s -> %v %d", tt.allow, tt.method, called, response.StatusCode)
			assert.Equalf(t, tt.wantStatus, response.StatusCode, "OnlyMethod: %s/%s -> %v %d", tt.allow, tt.method, called, response.StatusCode)
		})
	}
}

func TestRequestId(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		reqID string
		want  func(str string) bool
	}{
		{"send back", "kwa", func(str string) bool {
			return str == "kwa"
		}},
		{"generate", "", func(str string) bool {
			return len(str) > 0
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mw := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			req.Header.Set(HeaderRequestId, tt.reqID)
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			response := rr.Result()
			defer response.Body.Close()
			responseRequestID := response.Header.Get(HeaderRequestId)
			assert.Truef(t, tt.want(responseRequestID), "RequestID(%v) -> %v", tt.reqID, responseRequestID)
		})
	}
}

func TestRequestLogger(t *testing.T) {
	t.Parallel()
	logger := NewLogger("testing")
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(HeaderRequestId, "kwa")

	res := RequestLogger(r, logger)

	assert.NotEmpty(t, res, "RequestLogger(r, logger)")
}

func TestUnmarshalInput(t *testing.T) {
	t.Parallel()

	withContentTypeJson := func(r *http.Request) *http.Request {
		r.Header.Set(HeaderContentType, MediaTypeJson)
		return r
	}
	withContentTypeProtobuf := func(r *http.Request) *http.Request {
		r.Header.Set(HeaderContentType, MediaTypeProtobuf)
		return r
	}
	assertErrorContains := func(contains string) assert.ErrorAssertionFunc {
		return func(t assert.TestingT, err error, args ...interface{}) bool {
			return assert.ErrorContains(t, err, contains, args...)
		}
	}

	type args struct {
		r       *http.Request
		message any
	}
	tests := []struct {
		name    string
		args    args
		wantErr assert.ErrorAssertionFunc
		wantMsg func(*testing.T, any)
	}{
		{"no content type", args{httptest.NewRequest(http.MethodPost, "/top-up", strings.NewReader("")), nil}, assertErrorContains("content-type header required"), func(t *testing.T, message any) { assert.Nilf(t, message, "message want nil, got %v", message) }},
		{"ct json, body empty", args{withContentTypeJson(httptest.NewRequest(http.MethodPost, "/top-up", bytes.NewReader([]byte{}))), &proto.TopUpInput{}}, assertErrorContains("unmarshal json"), nil},
		{"ct json, body invalid", args{withContentTypeJson(httptest.NewRequest(http.MethodPost, "/top-up", strings.NewReader("xxx"))), &proto.TopUpInput{}}, assertErrorContains("unmarshal json"), nil},
		{"ct json, body invalid", args{withContentTypeJson(httptest.NewRequest(http.MethodPost, "/top-up", strings.NewReader("xxx"))), &proto.TopUpInput{}}, assertErrorContains("unmarshal json"), nil},
		{"ct json, top-up input", args{withContentTypeJson(httptest.NewRequest(http.MethodPost, "/top-up", strings.NewReader(`{"user_id": "anna","currency":"USD","value":"10.00"}`))), &proto.TopUpInput{}}, assert.NoError, func(t *testing.T, message any) {
			assert.Equalf(t, "anna", message.(*proto.TopUpInput).UserId, "message.user_id")
			assert.Equalf(t, "USD", message.(*proto.TopUpInput).Currency, "message.currency")
			assert.Equalf(t, "10.00", message.(*proto.TopUpInput).Value, "message.value")
			assert.Equalf(t, "", message.(*proto.TopUpInput).MerchantData, "message.merchant_data")
		}},

		{"ct protobuf, body empty", args{withContentTypeProtobuf(httptest.NewRequest(http.MethodPost, "/top-up", bytes.NewReader([]byte{}))), &proto.TopUpInput{}}, assert.NoError, func(t *testing.T, message any) {
			assert.Equalf(t, "", message.(*proto.TopUpInput).UserId, "message.user_id")
			assert.Equalf(t, "", message.(*proto.TopUpInput).Currency, "message.currency")
			assert.Equalf(t, "", message.(*proto.TopUpInput).Value, "message.value")
			assert.Equalf(t, "", message.(*proto.TopUpInput).MerchantData, "message.merchant_data")
		}},

		{"ct protobuf, body valid", args{withContentTypeProtobuf(httptest.NewRequest(http.MethodPost, "/top-up", bytes.NewReader([]byte{0xa, 0x6, 0x61, 0x73, 0x68, 0x6c, 0x65, 0x79, 0x12, 0x3, 0x54, 0x52, 0x59, 0x1a, 0x7, 0x31, 0x39, 0x35, 0x30, 0x2e, 0x30, 0x30, 0x22, 0xe, 0x7b, 0x22, 0x76, 0x61, 0x6c, 0x69, 0x64, 0x22, 0x3a, 0x74, 0x72, 0x75, 0x65, 0x7d}))), &proto.TopUpInput{}}, assert.NoError, func(t *testing.T, message any) {
			assert.Equalf(t, "ashley", message.(*proto.TopUpInput).UserId, "message.user_id")
			assert.Equalf(t, "TRY", message.(*proto.TopUpInput).Currency, "message.currency")
			assert.Equalf(t, "1950.00", message.(*proto.TopUpInput).Value, "message.value")
			assert.Equalf(t, `{"valid":true}`, message.(*proto.TopUpInput).MerchantData, "message.merchant_data")
		}},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			tt.wantErr(t, UnmarshalInput(tt.args.r, tt.args.message), fmt.Sprintf("UnmarshalInput(%v, %v)", tt.args.r, tt.args.message))
			if tt.wantMsg != nil {
				tt.wantMsg(t, tt.args.message)
			}
		})
	}
}

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/constb/tt-golang/internal/database"
	"github.com/constb/tt-golang/internal/proto"
	"github.com/constb/tt-golang/internal/utils"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	logger := utils.NewLogger("webservice")

	service, err := NewBalanceWebService(logger)
	if err != nil {
		logger.Panic("db init", zap.Error(err))
	}

	mux := http.NewServeMux()
	mux.Handle("top-up", service.TopUpHandler())

	server := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Panic("server serve", zap.Error(err))
		}
	}()

	utils.WaitForShutdownSignal()
	err = server.Shutdown(context.Background())
	if err != nil {
		logger.Panic("server shutdown", zap.Error(err))
	}
}

type BalanceWebService struct {
	logger *zap.Logger
	db     *database.BalanceDatabase
}

func NewBalanceWebService(logger *zap.Logger) (*BalanceWebService, error) {
	db, err := database.NewDatabaseConnection()
	if err != nil {
		return nil, err
	}

	return &BalanceWebService{logger, db}, nil
}

func (s *BalanceWebService) TopUpHandler() http.Handler {
	var handler http.Handler

	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := utils.RequestLogger(r, s.logger)

		input := proto.TopUpInput{}
		err := utils.UnmarshalInput(r, &input)
		if err != nil {
			logger.Error("bad input", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// top-up balance
		var output proto.GenericOutput
		var txId snowflake.ID
		txId, err = s.db.TopUp(r.Context(), input.UserId, input.Currency, input.Value, input.MerchantData)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if ok {
				logger.Info("top-up failed", zap.Error(err))
				output.Error = protoErr
			} else {
				logger.Error("top-up error", zap.Error(err))
				w.WriteHeader(500)
				return
			}
		} else {
			logger.Info("top-up new transaction", zap.Int64("txId", txId.Int64()))

			// return current balance data
			output.UserBalance = &proto.UserBalanceData{UserId: input.UserId}
			var available, reserved decimal.Decimal
			output.UserBalance.Currency, available, reserved, err = s.db.FetchUserBalance(r.Context(), input.UserId)
			if err != nil {
				protoErr, ok := err.(*proto.Error)
				if ok {
					logger.Info("fetch balance failed", zap.Error(err))
					output.Error = protoErr
					output.UserBalance = nil
				} else {
					logger.Error("fetch balance error", zap.Error(err))
					w.WriteHeader(500)
					return
				}
			} else {
				output.UserBalance.Value = available.StringFixedBank(2)
				output.UserBalance.ReservedValue = reserved.StringFixedBank(2)
				output.UserBalance.IsOverdraft = available.LessThan(decimal.Zero)
			}
		}
		utils.WriteOutput(r, w, logger, &output)
	})

	handler = utils.RequestId(handler)
	handler = utils.OnlyMethod(handler, http.MethodPost)
	handler = http.TimeoutHandler(handler, 5*time.Second, "")

	return handler
}

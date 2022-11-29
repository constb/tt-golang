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
	zap.RedirectStdLog(logger)

	db, err := database.NewDatabaseConnection()
	if err != nil {
		panic(err)
	}

	apiKey := os.Getenv("API_KEY")

	service, err := NewBalanceWebService(db, logger, apiKey)
	if err != nil {
		logger.Panic("db init", zap.Error(err))
	}

	mux := http.NewServeMux()
	mux.Handle("/top-up", service.TopUpHandler())
	mux.Handle("/reserve", service.ReserveHandler())
	mux.Handle("/commit", service.CommitHandler())

	server := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
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
	apiKey string
}

func NewBalanceWebService(db *database.BalanceDatabase, logger *zap.Logger, apiKey string) (*BalanceWebService, error) {
	return &BalanceWebService{logger, db, apiKey}, nil
}

func (s *BalanceWebService) TopUpHandler() http.Handler {
	var handler http.Handler

	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := utils.RequestLogger(r, s.logger)

		input := proto.TopUpInput{}
		err := utils.UnmarshalInput(r, &input)
		if err != nil {
			logger.Info("bad input", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// top-up balance
		var output proto.GenericOutput
		var txID snowflake.ID
		txID, err = s.db.TopUp(r.Context(), input.IdempotencyKey, input.UserId, input.Currency, input.Value, input.MerchantData)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if ok {
				logger.Info("top-up failed", zap.Error(err))
				output.Error = protoErr
				utils.WriteOutput(r, w, logger, &output)
			} else {
				logger.Error("top-up error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		logger.Info("new transaction (top-up)", zap.Int64("txID", txID.Int64()))

		// return current balance data
		output.UserBalance = &proto.UserBalanceData{UserId: input.UserId}
		var available, reserved decimal.Decimal
		output.UserBalance.Currency, available, reserved, err = s.db.FetchUserBalance(r.Context(), input.UserId)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if !ok {
				logger.Error("fetch balance error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			logger.Info("fetch balance failed", zap.Error(err))
			output.Error = protoErr
			output.UserBalance = nil
		} else {
			output.UserBalance.Value = available.StringFixedBank(2)
			output.UserBalance.ReservedValue = reserved.StringFixedBank(2)
			output.UserBalance.IsOverdraft = available.LessThan(decimal.Zero)
		}
		utils.WriteOutput(r, w, logger, &output)
	})

	handler = utils.ApiKey(handler, s.apiKey)
	handler = utils.RequestID(handler)
	handler = utils.OnlyMethod(handler, http.MethodPost)
	handler = http.TimeoutHandler(handler, 5*time.Second, "")

	return handler
}

func (s *BalanceWebService) ReserveHandler() http.Handler {
	var handler http.Handler

	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := utils.RequestLogger(r, s.logger)

		input := proto.ReserveInput{}
		err := utils.UnmarshalInput(r, &input)
		if err != nil {
			logger.Info("bad input", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// add reservation
		var output proto.GenericOutput
		err = s.db.Reserve(r.Context(), input.UserId, input.Currency, input.Value, input.OrderId, input.ItemId)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if ok {
				logger.Info("reservation failed", zap.Error(err))
				output.Error = protoErr
				utils.WriteOutput(r, w, logger, &output)
			} else {
				logger.Error("reservation error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		logger.Info("new reservation", zap.String("orderID", input.OrderId), zap.String("userID", input.UserId))

		// return current balance data
		output.UserBalance = &proto.UserBalanceData{UserId: input.UserId}
		var available, reserved decimal.Decimal
		output.UserBalance.Currency, available, reserved, err = s.db.FetchUserBalance(r.Context(), input.UserId)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if !ok {
				logger.Error("fetch balance error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			logger.Info("fetch balance failed", zap.Error(err))
			output.Error = protoErr
			output.UserBalance = nil
		} else {
			output.UserBalance.Value = available.StringFixedBank(2)
			output.UserBalance.ReservedValue = reserved.StringFixedBank(2)
			output.UserBalance.IsOverdraft = available.LessThan(decimal.Zero)
		}
		utils.WriteOutput(r, w, logger, &output)
	})

	handler = utils.ApiKey(handler, s.apiKey)
	handler = utils.RequestID(handler)
	handler = utils.OnlyMethod(handler, http.MethodPost)
	handler = http.TimeoutHandler(handler, 5*time.Second, "")

	return handler
}

func (s *BalanceWebService) CommitHandler() http.Handler {
	var handler http.Handler

	handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger := utils.RequestLogger(r, s.logger)

		input := proto.CommitReservationInput{}
		err := utils.UnmarshalInput(r, &input)
		if err != nil {
			logger.Info("bad input", zap.Error(err))
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// charge from balance
		var output proto.GenericOutput
		var txID snowflake.ID
		txID, err = s.db.CommitReservation(r.Context(), input.UserId, input.Currency, input.Value, input.OrderId, input.ItemId)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if ok {
				logger.Info("commit failed", zap.Error(err))
				output.Error = protoErr
				utils.WriteOutput(r, w, logger, &output)
			} else {
				logger.Error("commit error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		logger.Info("new transaction (charge)", zap.Int64("txID", txID.Int64()))

		// return current balance data
		output.UserBalance = &proto.UserBalanceData{UserId: input.UserId}
		var available, reserved decimal.Decimal
		output.UserBalance.Currency, available, reserved, err = s.db.FetchUserBalance(r.Context(), input.UserId)
		if err != nil {
			protoErr, ok := err.(*proto.Error)
			if !ok {
				logger.Error("fetch balance error", zap.Error(err))
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			logger.Info("fetch balance failed", zap.Error(err))
			output.Error = protoErr
			output.UserBalance = nil
		} else {
			output.UserBalance.Value = available.StringFixedBank(2)
			output.UserBalance.ReservedValue = reserved.StringFixedBank(2)
			output.UserBalance.IsOverdraft = available.LessThan(decimal.Zero)
		}
		utils.WriteOutput(r, w, logger, &output)
	})

	handler = utils.ApiKey(handler, s.apiKey)
	handler = utils.RequestID(handler)
	handler = utils.OnlyMethod(handler, http.MethodPost)
	handler = http.TimeoutHandler(handler, 5*time.Second, "")

	return handler
}

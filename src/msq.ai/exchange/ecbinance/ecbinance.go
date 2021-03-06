package ecbinance

import (
	"context"
	"fmt"
	"github.com/adshao/go-binance"
	log "github.com/sirupsen/logrus"
	"msq.ai/connectors/connector"
	"msq.ai/connectors/proto"
	"msq.ai/constants"
	"msq.ai/data/cmd"
	"msq.ai/utils/math"
	"strconv"
	"time"
)

const filledValue = "FILLED"
const expiredValue = "EXPIRED"
const orderNotExistError = -2013

func RunBinanceConnector(in <-chan *proto.ExecRequest, out chan<- *proto.ExecResponse, execPoolSize int) {

	ctxLog := log.WithFields(log.Fields{"id": "BinanceConnector"})

	orderToString := func(order *binance.CreateOrderResponse) string {

		if order == nil {
			return "nil"
		}

		var fill = ""

		if order.Fills != nil && len(order.Fills) > 0 && order.Fills[0] != nil {
			fill = fmt.Sprintf("%+v", order.Fills[0])
		}

		return fmt.Sprintf("%+v %s", order, fill)
	}

	errorResponse := func(response *proto.ExecResponse, err error) *proto.ExecResponse {

		response.Status = proto.StatusError

		response.Description = response.Description + " Parse error [" + err.Error() + "]"

		return response
	}

	trade := func(request *proto.ExecRequest, response *proto.ExecResponse) *proto.ExecResponse {

		client := binance.NewClient(request.RawCmd.ApiKey, request.RawCmd.SecretKey)

		orderService := client.NewCreateOrderService().Symbol(request.RawCmd.Instrument)
		orderService = orderService.NewClientOrderID(request.RawCmd.Id)

		if request.RawCmd.OrderType == constants.OrderTypeMarketName {
			orderService = orderService.Type(binance.OrderTypeMarket)
		} else if request.RawCmd.OrderType == constants.OrderTypeLimitName {
			orderService = orderService.Type(binance.OrderTypeLimit)
			orderService = orderService.Price(request.RawCmd.LimitPrice)
		} else {
			ctxLog.Fatal("Protocol violation! ExecRequest wrong OrderType ! ", request)
			return nil
		}

		if request.RawCmd.Direction == constants.OrderDirectionBuyName {
			orderService = orderService.Side(binance.SideTypeBuy)
		} else if request.RawCmd.Direction == constants.OrderDirectionSellName {
			orderService = orderService.Side(binance.SideTypeSell)
		} else {
			ctxLog.Fatal("Protocol violation! ExecRequest wrong Direction with empty cmd ! ", request)
			return nil
		}

		if request.RawCmd.OrderType == constants.OrderTypeLimitName {
			if request.RawCmd.TimeInForce == constants.TimeInForceGtcName {
				orderService = orderService.TimeInForce(binance.TimeInForceGTC)
			} else if request.RawCmd.TimeInForce == constants.TimeInForceFokName {
				orderService = orderService.TimeInForce(binance.TimeInForceFOK)
			} else {
				msg := "Protocol violation! ExecRequest has wrong TimeInForce."
				ctxLog.Error(msg, request)
				response.Description = msg
				return response
			}
		}

		orderService = orderService.Quantity(request.RawCmd.Amount)

		start := time.Now()

		order, err := orderService.Do(context.Background())

		response.OutsideExecution = time.Now().Sub(start)

		if err != nil {
			ctxLog.Error("Trade error ", err)
			response.Description = err.Error()
			return response
		}

		response.Description = orderToString(order)

		ctxLog.Trace("Order from Binance ", response.Description)

		if request.RawCmd.OrderType == constants.OrderTypeMarketName {

			if order.Status != filledValue {
				response.Description = "Order wasn't fill"
				return response
			}

		} else if request.RawCmd.OrderType == constants.OrderTypeLimitName {

			if order.Status == expiredValue {
				response.Description = "Order rejected"
				response.Status = proto.StatusRejected
				return response
			} else if order.Status != filledValue {
				response.Description = "Order wasn't fill"
				return response
			}

		} else {
			ctxLog.Fatal("Protocol violation! ExecRequest has wrong OrderType.", request)
		}

		response.Order = &cmd.Order{}

		response.Order.ExternalOrderId = order.OrderID

		response.Order.ExecutionId, err = strconv.ParseInt(order.ClientOrderID, 10, 64)

		if err != nil {
			return errorResponse(response, err)
		}

		response.Order.Price, err = strconv.ParseFloat(order.Fills[0].Price, 64)

		if err != nil {
			return errorResponse(response, err)
		}

		response.Order.Commission, err = strconv.ParseFloat(order.Fills[0].Commission, 64)

		if err != nil {
			return errorResponse(response, err)
		}

		response.Order.CommissionAsset = order.Fills[0].CommissionAsset

		response.Status = proto.StatusOk

		return response
	}

	check := func(request *proto.ExecRequest, response *proto.ExecResponse) *proto.ExecResponse {

		client := binance.NewClient(request.RawCmd.ApiKey, request.RawCmd.SecretKey)

		order, err := client.NewGetOrderService().Symbol(request.RawCmd.Instrument).OrigClientOrderID(request.RawCmd.Id).Do(context.Background())

		if err != nil {

			if binance.IsAPIError(err) && err.(*binance.APIError).Code == orderNotExistError {

				if request.Cmd.ExecuteTillTime.After(time.Now()) {
					return trade(request, response)
				} else {
					ctxLog.Info("Check error, order not exist and will be marked timed_out ", err)
					response.Description = err.Error()
					response.Status = proto.StatusTimedOut
					return response
				}

			} else {
				ctxLog.Error("Check error ", err)
				response.Description = err.Error()
				return response
			}
		}

		ctxLog.Trace("Order from Binance ", order)

		response.Description = fmt.Sprintf("%+v", order)

		if request.RawCmd.OrderType == constants.OrderTypeMarketName {

			if order.Status != filledValue {
				response.Description = "Order wasn't fill"
				return response
			}

		} else if request.RawCmd.OrderType == constants.OrderTypeLimitName {

			if order.Status == expiredValue {
				response.Description = "Order rejected"
				response.Status = proto.StatusRejected
				return response
			} else if order.Status != filledValue {
				response.Description = "Order wasn't fill"
				return response
			}

		} else {
			ctxLog.Fatal("Protocol violation! ExecRequest has wrong OrderType.", request)
		}

		response.Order = &cmd.Order{}

		response.Order.ExternalOrderId = order.OrderID

		response.Order.ExecutionId, err = strconv.ParseInt(order.ClientOrderID, 10, 64)

		if err != nil {
			return errorResponse(response, err)
		}

		response.Order.Price, err = strconv.ParseFloat(order.Price, 64)

		if err != nil {
			return errorResponse(response, err)
		}

		response.Order.Commission = 0

		response.Order.CommissionAsset = "UNKNOWN"

		response.Status = proto.StatusOk

		return response
	}

	info := func(request *proto.ExecRequest, response *proto.ExecResponse) *proto.ExecResponse {

		client := binance.NewClient(request.RawCmd.ApiKey, request.RawCmd.SecretKey)

		account, err := client.NewGetAccountService().Do(context.Background())

		if err != nil {
			ctxLog.Error("Info error ", err)
			response.Description = err.Error()
			return response
		}

		response.Balances = make([]cmd.Balance, 0)

		for _, b := range account.Balances {

			free, err := strconv.ParseFloat(b.Free, 64)

			if err != nil {
				return errorResponse(response, err)
			}

			locked, err := strconv.ParseFloat(b.Locked, 64)

			if err != nil {
				return errorResponse(response, err)
			}

			if !math.IsZero(free) || !math.IsZero(locked) {
				response.Balances = append(response.Balances, cmd.Balance{Asset: b.Asset, Free: free, Locked: locked})
			}
		}

		response.Status = proto.StatusOk

		return response
	}

	connector.RunConnector(ctxLog, in, out, execPoolSize, trade, check, info)
}

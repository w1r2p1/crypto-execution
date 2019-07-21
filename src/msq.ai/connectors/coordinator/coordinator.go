package coordinator

import (
	log "github.com/sirupsen/logrus"
	"msq.ai/connectors/proto"
	"msq.ai/constants"
	"msq.ai/data/cmd"
	"msq.ai/db/postgres/dao"
	dic "msq.ai/db/postgres/dictionaries"
	pgh "msq.ai/db/postgres/helper"
	"time"
)

func RunCoordinator(dburl string, dictionaries *dic.Dictionaries, out chan<- *proto.ExecRequest, in <-chan *proto.ExecResponse, exchangeId int16, connectorId int16) {

	ctxLog := log.WithFields(log.Fields{"id": "Coordinator"})

	ctxLog.Info("Coordinator is going to start")

	if len(dburl) < 1 {
		ctxLog.Fatal("dburl is empty !")
	}

	if in == nil {
		ctxLog.Fatal("ExecRequest channel is nil !")
	}

	if out == nil {
		ctxLog.Fatal("ExecResponse channel is nil !")
	}

	//------------------------------------------------------------------------------------------------------------------

	var pingTime = 30 * time.Second
	var prevOpTime time.Time
	var id int64 = 0

	incId := func() {
		id = id + 1
	}

	//------------------------------------------------------------------------------------------------------------------

	pingExchange := func() {

		for {

			incId()

			request := &proto.ExecRequest{Id: id, What: proto.ExecRequestCheckConnection}

			out <- request

			response := <-in

			if response == nil {
				ctxLog.Fatal("Protocol violation! ExecResponse is nil")
			}

			if response.Id != request.Id {
				ctxLog.Fatal("Protocol violation! response.Id doesn't equal request.Id")
			}

			if response.Status == proto.ExecResponseStatusOk {
				prevOpTime = time.Now()
				log.Info("Exchange successfully pinged")
				return
			} else if response.Status == proto.ExecResponseStatusError {
				log.Info("Exchange ping error ! ", response.Description)
			} else {
				ctxLog.Fatal("Protocol violation! Ping response has unknown status")
			}

			time.Sleep(5 * time.Second)
		}
	}

	//------------------------------------------------------------------------------------------------------------------

	dump := make(chan *proto.ExecResponse, 10)

	go func() {

		db, err := pgh.GetDbByUrl(dburl)

		if err != nil {
			ctxLog.Fatal("Cannot connect to DB with URL ["+dburl+"] ", err)
		}

		db.SetMaxIdleConns(1)
		db.SetMaxOpenConns(3)
		db.SetConnMaxLifetime(time.Hour)

		for {

			response := <-dump

			ctxLog.Trace("Dump execution result to DB ", response)

			// TODO !!!!!!!!!!!!!!!!!!
		}

	}()

	//------------------------------------------------------------------------------------------------------------------

	go func() {

		db, err := pgh.GetDbByUrl(dburl)

		if err != nil {
			ctxLog.Fatal("Cannot connect to DB with URL ["+dburl+"] ", err)
		}

		db.SetMaxIdleConns(1)
		db.SetMaxOpenConns(3)
		db.SetConnMaxLifetime(time.Hour)

		dbTryGetCommandForExecution := func() *cmd.Command {

			statusCreatedId := dictionaries.ExecutionStatuses().GetIdByName(constants.ExecutionStatusCreatedName)
			statusExecutingId := dictionaries.ExecutionStatuses().GetIdByName(constants.ExecutionStatusExecutingName)

			// TODO TIME !!!!

			result, err := dao.TryGetCommandForExecution(db, exchangeId, connectorId, time.Now(), statusCreatedId, statusExecutingId)

			if err != nil {
				ctxLog.Error("dbTryGetCommandForExecution error ! ", err)
				time.Sleep(5 * time.Second)
				return nil
			}

			return result
		}

		var command *cmd.Command

		for {

			// TODO restore state lost operations

			// TODO try get from DB

			command = dbTryGetCommandForExecution()

			if command != nil {

				// TODO execute

				// TODO save result to DB
			} else {

				delta := time.Now().Sub(prevOpTime)

				if delta > pingTime {
					pingExchange()
				}

				time.Sleep(250 * time.Millisecond)
			}
		}

	}()

}

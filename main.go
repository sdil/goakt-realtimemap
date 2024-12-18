package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	vehicle "sdil-busmap/gen/protos"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/log"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func createVehicleHandler(actorSystem goakt.ActorSystem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid := r.URL.Query().Get("id")

		pid, err := actorSystem.LocalActor(vid)
		if err != nil {
			fmt.Fprintf(w, "Error: %v", err)
			return
		}
		command := &vehicle.GetPosition{}
		res, _ := goakt.Ask(context.Background(), pid, command, time.Minute)
		position := res.(*vehicle.GetPosition)

		fmt.Fprintf(w, "pid %v, latitude: %v, longitude: %v", pid.Name(), position.Latitude, position.Longitude)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Reply struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

func createVehicleWsHandler(actorSystem goakt.ActorSystem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Print("upgrade err:", err)
			return
		}
		defer ws.Close()

		// var locationHistory LocationHistoryRequest
		// err = ws.ReadJSON(&locationHistory)
		// if err != nil {
		// 	if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
		// 		fmt.Printf("Read error: %v\n", err)
		// 	}
		// 	return
		// }

		// if locationHistory.Type == "locationHistory" {
		// 	pid, _ := actorSystem.LocalActor(locationHistory.Id)
		// 	command := &vehicle.GetPositionHistory{}
		// 	res, err := goakt.Ask(context.Background(), pid, command, time.Second)
		// 	if err != nil {
		// 		fmt.Printf("Error getting position history: %v\n", err)
		// 		return
		// 	}
		// 	descriptors := res.ProtoReflect().Descriptor().Fields()
		// 	positions := res.ProtoReflect().Get(descriptors.ByName("positions")).List()

		// 	locationReponse := LocationHistoryResponse{}
		// 	var positionsList []Location

		// 	for i := 0; i < positions.Len(); i++ {
		// 		position := positions.Get(i).Message()
		// 		fmt.Println(position)
		// 		latitude := position.Get(descriptors.ByName("latitude"))
		// 		fmt.Println(latitude)
		// 		longitude := position.Get(descriptors.ByName("latitude")).Float()
		// 		fmt.Println(longitude)

		// 		positionsList = append(positionsList, Location{
		// 			Latitude:  latitude.Float(),
		// 			Longitude: longitude,
		// 		})
		// 	}
		// 	locationReponse.Positions = positionsList
		// 	fmt.Println(locationReponse)
		// 	ws.WriteJSON(Reply{
		// 		Type: "locationHistory",
		// 		Data: locationReponse,
		// 	})
		// }

		for {
			for _, pid := range actorSystem.Actors() {
				command := &vehicle.GetPosition{}
				res, _ := goakt.Ask(context.Background(), pid, command, time.Second)

				position, ok := res.(*vehicle.GetPosition)
				if !ok {
					fmt.Println("Error getting position")
					return
				}

				err = ws.WriteJSON(Reply{
					Type: "vehiclePosition",
					Data: position,
				})
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						fmt.Println("write:", err)
					}
					fmt.Println(err)
					ws.Close()
					return
				}
			}
		}
	}
}

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	// Connect to SQLite database
	db, err := sql.Open("sqlite3", "vehicle_position.db?_journal_mode=WAL")
	if err != nil {
		logger.Error("Error connecting to SQLite database", err)
		return
	}
	defer db.Close()

	// Verify the connection
	err = db.Ping()
	if err != nil {
		logger.Error("Error verifying connection to SQLite database", err)
		return
	}

	logger.Info("Successfully connected to SQLite database")

	actorSystem, err := goakt.NewActorSystem("VehicleActorSystem",
		goakt.WithPassivationDisabled(),
		goakt.WithLogger(logger),
		goakt.WithActorInitMaxRetries(3),
	)
	if err != nil {
		logger.Error("Error creating actor system", err)
		return
	}

	err = actorSystem.Start(ctx)
	if err != nil {
		logger.Error("Error starting actor system", err)
		return
	}

	// http.HandleFunc("/", homeHandler)
	http.HandleFunc("/realtime-vehicle", createVehicleWsHandler(actorSystem))
	http.HandleFunc("/vehicle", createVehicleHandler(actorSystem))

	fmt.Println("Server is starting on port 8080...")
	go func() {
		host := "localhost:8080"
		err = http.ListenAndServe(host, nil)
		if err != nil {
			fmt.Printf("Error starting server: %s\n", err)
		}
	}()

	go func() {
		ingressDone := ConsumeVehicleEvents(func(event *Event) {
			if event.VehiclePosition.HasValidPosition() {
				vid := &event.VehicleId

				pid, err := actorSystem.Spawn(ctx, *vid, NewVehicle(*vid, db))
				if err != nil {
					logger.Error("Error starting actor instance", err)
					return
				}

				command := &vehicle.UpdatePosition{
					Latitude:  *event.VehiclePosition.Latitude,
					Longitude: *event.VehiclePosition.Longitude,
				}

				_ = goakt.Tell(ctx, pid, command)
			}
		}, ctx)

		<-ingressDone
	}()

	go func() {
		for {
			actors := actorSystem.Actors()
			for _, actor := range actors {
				actorSystem.Schedule(ctx, &vehicle.PersistLocation{}, actor, time.Minute)
			}
			time.Sleep(time.Minute)
		}
	}()

	// Capture ctr+c signal
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	logger.Info("Shutting down actor system")
	err = actorSystem.Stop(ctx)
	if err != nil {
		logger.Error("Error stopping actor system", err)
		return
	}
}

package main

import (
	"context"
	"fmt"
	"net/http"
	vehicle "sdil-busmap/gen/protos"
	"time"

	"github.com/gorilla/websocket"
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
		res, _ := goakt.Ask(context.Background(), pid, command, time.Second)
		descriptors := res.ProtoReflect().Descriptor().Fields()

		latitude := res.ProtoReflect().Get(descriptors.ByName("latitude"))
		longitude := res.ProtoReflect().Get(descriptors.ByName("longitude"))

		fmt.Fprintf(w, "latitude: %v, longitude: %v", latitude, longitude)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all origins
	},
}

func createVehicleWsHandler(actorSystem goakt.ActorSystem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Print("upgrade:", err)
			return
		}
		defer ws.Close()

		done := make(chan struct{})

		go func() {
			defer close(done)
			for {
				_, _, err := ws.ReadMessage()
				if err != nil {
					fmt.Println("read:", err)
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						fmt.Printf("Read error: %v\n", err)
					}
					break
				}

			}
		}()

		for {
			for _, pid := range actorSystem.Actors() {
				command := &vehicle.GetPosition{}
				res, _ := goakt.Ask(context.Background(), pid, command, time.Second)
				descriptors := res.ProtoReflect().Descriptor().Fields()

				id := res.ProtoReflect().Get(descriptors.ByName("vehicle_id"))
				latitude := res.ProtoReflect().Get(descriptors.ByName("latitude"))
				longitude := res.ProtoReflect().Get(descriptors.ByName("longitude"))

				msg := fmt.Sprintf("{\"id\": \"%v\", \"latitude\": %v, \"longitude\": %v}", id, latitude, longitude)
				err = ws.WriteMessage(websocket.TextMessage, []byte(msg)) // mt TextMessage = 1
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						fmt.Println("write:", err)
					}
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

	actorSystem, err := goakt.NewActorSystem("VehicleActorSystem",
		goakt.WithPassivationDisabled(),
		goakt.WithLogger(logger),
		goakt.WithMailboxSize(1_000_000),
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

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/realtime-vehicle", createVehicleWsHandler(actorSystem))
	http.HandleFunc("/vehicle", createVehicleHandler(actorSystem))

	fmt.Println("Server is starting on port 8080...")
	go func() {
		err = http.ListenAndServe("localhost:8080", nil)
		if err != nil {
			fmt.Printf("Error starting server: %s\n", err)
		}
	}()

	ingressDone := ConsumeVehicleEvents(func(event *Event) {
		if event.VehiclePosition.HasValidPosition() {
			vid := &event.VehicleId

			// actors := actorSystem.Actors()
			_, pid, err := actorSystem.ActorOf(ctx, *vid)
			if err != nil {
				pid, err = actorSystem.Spawn(ctx, *vid, NewVehicle())
				if err != nil {
					logger.Error("Error starting actor instance", err)
					return
				}
			}

			command := &vehicle.UpdatePosition{
				Latitude:  *event.VehiclePosition.Latitude,
				Longitude: *event.VehiclePosition.Longitude,
			}

			_ = goakt.Tell(ctx, pid, command)
		}
	}, ctx)

	<-ingressDone
}

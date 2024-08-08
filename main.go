package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
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

type LocationMessage struct {
	Id        string  `json:"id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

func createVehicleWsHandler(actorSystem goakt.ActorSystem) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			fmt.Print("upgrade:", err)
			return
		}
		defer ws.Close()

		for {
			for _, pid := range actorSystem.Actors() {
				command := &vehicle.GetPosition{}
				res, _ := goakt.Ask(context.Background(), pid, command, time.Second)
				descriptors := res.ProtoReflect().Descriptor().Fields()

				id := res.ProtoReflect().Get(descriptors.ByName("vehicle_id"))
				latitude := res.ProtoReflect().Get(descriptors.ByName("latitude"))
				longitude := res.ProtoReflect().Get(descriptors.ByName("longitude"))

				msg := LocationMessage{
					Id: id.String(),
					Latitude: latitude.Float(),
					Longitude: longitude.Float(),
				}

				err = ws.WriteJSON(msg)
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
		host := "localhost:8080"
		if (os.Getenv("RENDER") == "true") {
			host = "0.0.0.0:10000"
		}
		err = http.ListenAndServe(host, nil)
		if err != nil {
			fmt.Printf("Error starting server: %s\n", err)
		}
	}()

	ingressDone := ConsumeVehicleEvents(func(event *Event) {
		if event.VehiclePosition.HasValidPosition() {
			vid := &event.VehicleId

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

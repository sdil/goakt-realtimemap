package main

import (
	"context"
	"fmt"
	"net/http"
	vehicle "sdil-busmap/gen/protos"
	"time"

	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/log"
)

func healthHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK!")
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

func main() {
	ctx := context.Background()
	logger := log.DefaultLogger

	actorSystem, err := goakt.NewActorSystem("VehicleActorSystem",
		goakt.WithPassivationDisabled(),
		goakt.WithLogger(logger),
		goakt.WithMailboxSize(1_000_000),
		goakt.WithActorInitMaxRetries(3),
	)

	err = actorSystem.Start(ctx)
	if err != nil {
		logger.Error("Error starting actor system", err)
		return
	}

	http.HandleFunc("/", healthHandler)
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

package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	_ "github.com/mattn/go-sqlite3"
	"github.com/tochemey/goakt/v2/actors"
	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/address"
	"github.com/tochemey/goakt/v2/discovery/static"
	"github.com/tochemey/goakt/v2/log"

	pb "sdil-busmap/pb"
)

func homeHandler(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "index.html")
}

func createVehicleHandler(actorSystem goakt.ActorSystem, remoting goakt.Remoting) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vid := r.URL.Query().Get("id")
		logger := actorSystem.Logger()

		command := &pb.GetPosition{}
		var position *pb.GetPosition

		addr, pid, err := actorSystem.ActorOf(r.Context(), vid)
		if err != nil {
			logger.Error("Error sending command to actor", err)
			fmt.Fprintf(w, "Error sending command to actor %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		switch {
		case errors.Is(err, actors.ErrActorNotFound(vid)):
			fmt.Fprintf(w, "vid %v not found", vid)
			w.WriteHeader(http.StatusNotFound)
			return
		case pid != nil:
			res, err := pid.SendSync(r.Context(), vid, command, time.Minute)
			if err != nil || res == nil {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			position = res.(*pb.GetPosition)
		case addr != nil:
			res, _ := remoting.RemoteAsk(r.Context(), address.NoSender(), addr, command, time.Minute)
			unmarshalled, err := res.UnmarshalNew()
			if err != nil || res == nil  {
				logger.Info("Failed to unmarshall")
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			position = unmarshalled.(*pb.GetPosition)
		}

		fmt.Fprintf(w, "vid %v, latitude: %v, longitude: %v", vid, position.Latitude, position.Longitude)
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

type Reply struct {
	Id       string      `json:"id"`
	Type     string      `json:"type"`
	Position interface{} `json:"position"`
}

func createVehicleWsHandler(actorSystem goakt.ActorSystem) http.HandlerFunc {
	logger := actorSystem.Logger()
	return func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			logger.Errorf("upgrade error: %v", err)
			return
		}

		defer func(ws *websocket.Conn) {
			err := ws.Close()
			if err != nil {
				logger.Errorf("close error: %v", err)
			}
		}(ws)

		for {
			// TODO: refactor this code because Actors is expensive call and we need to have a way to keep track of specific actors
			// There are system actors that do not understand the GetPosition command
			for _, pid := range actorSystem.Actors() {
				// TODO: revisit this design
				// In the meantime we cannot just ask all the the actors in the system
				// we need to narrow it to actor types
				switch pid.Actor().(type) {
				case *Vehicle:
				// pass
				default:
					continue
				}

				command := &pb.GetPosition{}

				res, _ := pid.SendSync(r.Context(), pid.Name(), command, time.Minute)

				position, ok := res.(*pb.GetPosition)
				if !ok {
					logger.Error("failed to get position")
					return
				}

				err = ws.WriteJSON(Reply{
					Id:       pid.Name(),
					Type:     "vehiclePosition",
					Position: position,
				})

				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						logger.Error(fmt.Errorf("%w", err))
					}
					logger.Errorf("websocket error: %w", err)
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

	logger.Info("Starting the Goakt cluster")
	GossipPort := 3322
	PeersPort := 3320
	var RemotingPort int32 = 50052

	// define the discovery options
	discoConfig := static.Config{
		Hosts: []string{
			fmt.Sprintf("node0:%d", GossipPort),
			fmt.Sprintf("node1:%d", GossipPort),
			fmt.Sprintf("node2:%d", GossipPort),
		},
	}
	// instantiate the dnssd discovery provider
	disco := static.NewDiscovery(&discoConfig)

	// grab the host
	host, _ := os.Hostname()

	clusterConfig := goakt.
		NewClusterConfig().
		WithDiscovery(disco).
		WithPartitionCount(19).
		WithDiscoveryPort(GossipPort).
		WithPeersPort(PeersPort).
		WithKinds(new(Vehicle))

	isRunningOnContainer := os.Getpid() == 1

	var actorSystem goakt.ActorSystem

	if isRunningOnContainer {
		logger.Info("Running in container with cluster mode")
		actorSystem, err = goakt.NewActorSystem("VehicleActorSystem",
			goakt.WithPassivationDisabled(),
			goakt.WithActorInitMaxRetries(3),
			goakt.WithRemoting(host, RemotingPort),
			goakt.WithCluster(clusterConfig),
		)
	} else {
		logger.Info("Running in local mode")
		actorSystem, err = goakt.NewActorSystem("VehicleActorSystem",
			goakt.WithPassivationDisabled(),
			goakt.WithLogger(logger),
			goakt.WithActorInitMaxRetries(3),
		)
	}

	if err != nil {
		logger.Error("Error creating actor system", err)
		return
	}

	// TODO: Consider using Run() function for better error handling
	// and coordinated shutdown
	err = actorSystem.Start(ctx)
	if err != nil {
		logger.Error("Error starting actor system", err)
		return
	}
	defer func() {
		logger.Info("Shutting down actor system")
		err := actorSystem.Stop(ctx)
		if err != nil {
			logger.Error("Error stopping actor system", err)
		}
	}()

	remoting := actors.NewRemoting()

	http.HandleFunc("/", homeHandler)
	http.HandleFunc("/realtime-vehicle", createVehicleWsHandler(actorSystem))
	http.HandleFunc("/vehicle", createVehicleHandler(actorSystem, *remoting))

	logger.Info("Server is starting on port 8080...")
	go func() {
		host := "localhost:8082"
		if isRunningOnContainer {
			host = "0.0.0.0:8080"
		}
		err = http.ListenAndServe(host, nil)
		if err != nil {
			fmt.Printf("Error starting server: %s\n", err)
		}
	}()

	go func() {
		ingressDone := ConsumeVehicleEvents(func(event *Event) {
			if event.VehiclePosition.HasValidPosition() {
				vid := &event.VehicleId

				command := &pb.UpdatePosition{
					Latitude:  *event.VehiclePosition.Latitude,
					Longitude: *event.VehiclePosition.Longitude,
				}

				addr, pid, err := actorSystem.ActorOf(ctx, *vid)

				switch {
				case err != nil:
					logger.Infof("Starting actor instance %v on node %v", *vid, actorSystem.Host())
					// If actor is not found, create a new one
					pid, err = actorSystem.Spawn(ctx,
						*vid,
						NewVehicle(*vid, db),
						goakt.WithSupervisorStrategies(
							goakt.NewSupervisorStrategy(
								goakt.InternalError{},
								goakt.NewRestartDirective()),
						),
					)
					if err != nil {
						logger.Error("Error starting actor instance", err)
						return
					}
					_ = pid.SendAsync(ctx, *vid, command)
				case pid != nil:
					_ = pid.SendAsync(ctx, *vid, command)
				case addr != nil:
					_ = remoting.RemoteTell(ctx, address.NoSender(), addr, command)
				}
			}
		}, ctx)

		<-ingressDone
	}()

	// Let the events flow for a minute before scheduling persist location
	go func() {
		time.Sleep(time.Minute)
		logger.Debug("scheduling persist location")

		// TODO: refactor this code because Actors is expensive call and we need to have a way to keep track of specific actors
		actors := actorSystem.Actors()
		for _, actor := range actors {
			switch actor.Actor().(type) {
			case *Vehicle:
			// pass
			default:
				continue
			}

			// Distribute the update randomly
			// so that the database load is not too high
			randomNumber := rand.Intn(60) + 1
			err := actorSystem.ScheduleWithCron(ctx, &pb.PersistLocation{}, actor, fmt.Sprintf("%d * * * * * *", randomNumber))
			if err != nil {
				logger.Error("Error scheduling persist location", err)
				return
			}
		}
	}()

	// Capture ctr+c signal
	interruptSignal := make(chan os.Signal, 1)
	signal.Notify(interruptSignal, os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	<-interruptSignal

	// make sure if it is unix init process to exit
	if isRunningOnContainer {
		os.Exit(0)
	}

	process, _ := os.FindProcess(os.Getpid())
	switch {
	case runtime.GOOS == "windows":
		_ = process.Kill()
	default:
		_ = process.Signal(syscall.SIGTERM)
	}
}

package main

import (
	"context"
	"database/sql"
	"fmt"
	pb "sdil-busmap/pb"
	"time"

	goakt "github.com/tochemey/goakt/v3/actor"
	"github.com/tochemey/goakt/v3/goaktpb"
	"github.com/tochemey/goakt/v3/log"
)

type Position struct {
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

type Vehicle struct {
	id       string
	position []Position
	db       *sql.DB
	logger   log.Logger
}

// ensure that Vehicle implements Actor interface
var _ goakt.Actor = (*Vehicle)(nil)

func NewVehicle(id string, db *sql.DB) *Vehicle {
	return &Vehicle{
		id: id,
		db: db,
	}
}

func (v *Vehicle) PreStart(ctx *goakt.Context) error {
	v.position = make([]Position, 0)
	return nil
}

func (v *Vehicle) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		v.logger = ctx.Logger()
		v.logger.Infof("Vehicle=(%s) started", v.id)
	case *pb.GetPosition:
		ctx.Response(&pb.GetPosition{
			Latitude:  v.position[len(v.position)-1].Latitude,
			Longitude: v.position[len(v.position)-1].Longitude,
		})
	case *pb.UpdatePosition:
		pos := Position{
			Latitude:  msg.GetLatitude(),
			Longitude: msg.GetLongitude(),
			Timestamp: time.Now(),
		}
		v.position = append(v.position, pos)
	case *pb.GetPositionHistory:
		positions := make([]*pb.GetPosition, 0)
		for _, p := range v.position {
			positions = append(positions, &pb.GetPosition{
				Latitude:  p.Latitude,
				Longitude: p.Longitude,
			})
		}
		ctx.Response(&pb.GetPositionHistory{
			Positions: positions,
		})
	case *pb.PersistLocation:
		if err := v.persistLocation(ctx.Context()); err != nil {
			v.logger.Errorf("failed to persist location: %v", err)
		}

	default:
		ctx.Unhandled()
	}
}

func (v *Vehicle) PostStop(ctx *goakt.Context) error {
	return v.persistLocation(ctx.Context())
}

func (v *Vehicle) persistLocation(ctx context.Context) error {
	v.logger.Debugf("Vehicle=(%s) persisting location", v.id)
	_, err := v.db.ExecContext(ctx, "INSERT INTO positions (id, latitude, longitude, timestamp) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET latitude = excluded.latitude, longitude = excluded.longitude, timestamp = excluded.timestamp",
		v.id,
		v.position[len(v.position)-1].Latitude,
		v.position[len(v.position)-1].Longitude,
		v.position[len(v.position)-1].Timestamp,
	)
	if err != nil {
		return fmt.Errorf("failed to persist location: %v", err)
	}
	v.logger.Debugf("Vehicle=(%s) location successfully persisted", v.id)
	return nil
}

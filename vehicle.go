package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	vehicle "sdil-busmap/gen/protos"

	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
)

type Vehicle struct {
	id       string
	position []Position
	db       *sql.DB
}

type Position struct {
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

func NewVehicle(id string, db *sql.DB) *Vehicle {
	return &Vehicle{
		id: id,
		db: db,
	}
}

func (v *Vehicle) PreStart(ctx context.Context) error {
	v.position = make([]Position, 0)
	return nil
}

func (v *Vehicle) Receive(ctx *goakt.ReceiveContext) {
	switch msg := ctx.Message().(type) {
	case *goaktpb.PostStart:
		fmt.Println("Vehicle", v.id, "started")
	case *vehicle.GetPosition:
		ctx.Response(&vehicle.GetPosition{
			Latitude:  v.position[len(v.position)-1].Latitude,
			Longitude: v.position[len(v.position)-1].Longitude,
		})
	case *vehicle.UpdatePosition:
		pos := Position{
			Latitude:  msg.GetLatitude(),
			Longitude: msg.GetLongitude(),
			Timestamp: time.Now(),
		}
		v.position = append(v.position, pos)
	case *vehicle.GetPositionHistory:
		positions := make([]*vehicle.GetPosition, 0)
		for _, p := range v.position {
			positions = append(positions, &vehicle.GetPosition{
				Latitude:  p.Latitude,
				Longitude: p.Longitude,
			})
		}
		ctx.Response(&vehicle.GetPositionHistory{
			Positions: positions,
		})
	case *vehicle.PersistLocation:
		err := v.persistLocation()
		if err != nil {
			fmt.Println("Error persisting location", err)
		}
	default:
		ctx.Unhandled()
	}
}

func (v *Vehicle) PostStop(ctx context.Context) error {
	v.persistLocation()
	return nil
}

func (v *Vehicle) persistLocation() error {
	fmt.Println("Persisting location", v.id)
	_, err := v.db.Exec("INSERT INTO positions (id, latitude, longitude, timestamp) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET latitude = excluded.latitude, longitude = excluded.longitude, timestamp = excluded.timestamp",
		v.id,
		v.position[len(v.position)-1].Latitude,
		v.position[len(v.position)-1].Longitude,
		v.position[len(v.position)-1].Timestamp,
	)
	if err != nil {
		fmt.Println("Error inserting location", err)
		return err
	}
	return nil
}

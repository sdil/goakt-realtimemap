package main

import (
	"context"
	"fmt"
	vehicle "sdil-busmap/gen/protos"
	"time"

	goakt "github.com/tochemey/goakt/v2/actors"
	"github.com/tochemey/goakt/v2/goaktpb"
)

type Vehicle struct {
	id       string
	position []Position
}

type Position struct {
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

func NewVehicle() *Vehicle {
	return &Vehicle{}
}

func (v *Vehicle) PreStart(ctx context.Context) error {
	v.position = make([]Position, 0)
	return nil
}

func (v *Vehicle) Receive(ctx *goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		v.id = ctx.Self().Name()
		fmt.Println("Vehicle", v.id, "started")
	case *vehicle.GetPosition:
		ctx.Response(&vehicle.GetPosition{
			Latitude:  v.position[len(v.position)-1].Latitude,
			Longitude: v.position[len(v.position)-1].Longitude,
		})
	case *vehicle.UpdatePosition:
		pos := Position{
			Latitude:  ctx.Message().(*vehicle.UpdatePosition).Latitude,
			Longitude: ctx.Message().(*vehicle.UpdatePosition).Longitude,
			Timestamp: time.Now(),
		}
		fmt.Println("update position", v.id, pos)
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
	default:
		ctx.Unhandled()
	}
}

func (v *Vehicle) PostStop(ctx context.Context) error {
	// Do nothing
	return nil
}

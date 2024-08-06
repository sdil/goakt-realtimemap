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
	id string
	position Position
}

type Position struct {
	Latitude float64
	Longitude float64
	Timestamp time.Time
}

func NewVehicle() *Vehicle {
	return &Vehicle{}
}

func (v *Vehicle) PreStart(ctx context.Context) error {
	v.position = Position{}
	return nil
}

func (v *Vehicle) Receive(ctx goakt.ReceiveContext) {
	switch ctx.Message().(type) {
	case *goaktpb.PostStart:
		v.id = ctx.Self().Name()
	case *vehicle.GetPosition:
		ctx.Response(&vehicle.GetPosition{
			VehicleId: v.id,
			Latitude: v.position.Latitude,
			Longitude: v.position.Longitude,
		})
	case *vehicle.UpdatePosition:
		v.position = Position{
			Latitude: ctx.Message().(*vehicle.UpdatePosition).Latitude,
			Longitude: ctx.Message().(*vehicle.UpdatePosition).Longitude,
			Timestamp: time.Now(),
		}
		fmt.Printf("%v: Updating position %v\n", v.id, v.position)
	default:
		ctx.Unhandled()
	}
}

func (v *Vehicle) PostStop(ctx context.Context) error {
	// Do nothing
	return nil
}
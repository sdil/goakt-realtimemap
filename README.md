# Vehicle Tracking System

This project is a vehicle tracking system that uses WebSockets for real-time updates and a SQLite database for storing vehicle positions. The system is built using Go and leverages the `goakt` actor framework for handling vehicle actors.

Quick UI demo:

https://github.com/user-attachments/assets/09958842-ca17-4b07-8733-e0b6295be5db

## Features

- **Real-Time Tracking**: Uses WebSockets to provide real-time updates of vehicle positions.
- **Actor Model**: Utilizes the `goakt` actor framework to manage vehicle actors efficiently.
- **SQLite Database**: Stores vehicle positions in a SQLite database for persistence.
- **Buffered data persistence**: The data is persisted on database every 1 minute, and is distributed in a span of 1 minute to reduce the load on the database
- **REST API**: Provides a REST API to query the current position of vehicles.
- **WebSocket API**: Offers a WebSocket endpoint for real-time vehicle position updates.
- **Frontend Interface**: Includes an `index.html` file to visualize vehicle positions on a map.
- **Scalable Architecture**: Designed to handle multiple vehicle actors concurrently.

## Tech Stack

- **Programming Language**: Go
- **Database**: SQLite
- **WebSockets**: For real-time updates
- **Actor Framework**: `goakt`
- **Frontend**: HTML, JavaScript
- **Protocol Buffers**: For message serialization

## Getting Started

### Prerequisites

- Go 1.23 or later
- SQLite3

### Installation

1. Clone the repository:
    ```sh
    git clone https://github.com/sdil/goakt-realtimemap.git
    cd sdil/goakt-realtimemap
    ```

2. Install dependencies:
    ```sh
    go mod tidy
    ```

3. Run database migrations:
    ```sh
    sqlite3 vehicle_position.db < migrations.sql
    ```

### Running the Application in local mode

1. Start the server:
    ```sh
    go run main.go
    ```

2. Open http://localhost:8080 in your browser to view the real-time vehicle tracking map.
3. Curl http://localhost:8080/vehicle?id={vehicle_id} in your terminal to see location of individual vehicle.

### Running the Application in cluster mode

1. Build the container
    ```sh
    docker compose build
    ```
2. Start the containers
   ```sh
   docker compose up
   ```

### API Endpoints

- **GET /vehicle?id={vehicle_id}**: Get the current position of a vehicle.
- **WebSocket /realtime-vehicle**: Real-time updates of vehicle positions.

### Project Components

- **main.go**: Entry point of the application. Sets up the HTTP server and WebSocket handlers.
- **vehicle.go**: Defines the `Vehicle` actor and its behavior.
- **index.html**: Frontend for displaying the real-time vehicle positions on a map.
- **protos/vehicle.proto**: Protocol Buffers definition for vehicle messages.
- **gen/protos/vehicle.pb.go**: Generated Go code from the Protocol Buffers definition.

### Contributing

Contributions are welcome! Please open an issue or submit a pull request.
This repo uses [Earthly](https://earthly.dev/get-earthly) to generate the pbs. Once earthly install just run the following command

```bash
earthly --no-cache +protogen
```

### License

This project is licensed under the MIT License.
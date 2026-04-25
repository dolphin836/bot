#!/bin/bash

APP_NAME="bot"
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
APP_BIN="$APP_DIR/$APP_NAME"
PID_FILE="$APP_DIR/$APP_NAME.pid"
LOG_FILE="$APP_DIR/data/$APP_NAME.log"

# Ensure data directory exists
mkdir -p "$APP_DIR/data"

build() {
    echo "Building..."
    cd "$APP_DIR" && go build -o "$APP_BIN" ./cmd/bot/
    echo "Build complete: $APP_BIN"
}

start() {
    if is_running; then
        echo "$APP_NAME is already running (PID: $(cat "$PID_FILE"))"
        return 1
    fi

    if [ ! -f "$APP_BIN" ]; then
        echo "Binary not found, building first..."
        build
    fi

    echo "Starting $APP_NAME..."
    cd "$APP_DIR" && nohup "$APP_BIN" >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 1

    if is_running; then
        echo "$APP_NAME started (PID: $(cat "$PID_FILE"))"
        echo "Logs: $LOG_FILE"
    else
        echo "Failed to start $APP_NAME. Check logs: $LOG_FILE"
        rm -f "$PID_FILE"
        return 1
    fi
}

stop() {
    if ! is_running; then
        echo "$APP_NAME is not running"
        rm -f "$PID_FILE"
        return 0
    fi

    local pid
    pid=$(cat "$PID_FILE")
    echo "Stopping $APP_NAME (PID: $pid)..."
    kill "$pid"

    # Wait up to 10 seconds for graceful shutdown
    for i in $(seq 1 10); do
        if ! kill -0 "$pid" 2>/dev/null; then
            echo "$APP_NAME stopped"
            rm -f "$PID_FILE"
            return 0
        fi
        sleep 1
    done

    echo "Force killing $APP_NAME..."
    kill -9 "$pid" 2>/dev/null
    rm -f "$PID_FILE"
    echo "$APP_NAME killed"
}

restart() {
    stop
    start
}

rebuild() {
    stop
    build
    start
}

status() {
    if is_running; then
        echo "$APP_NAME is running (PID: $(cat "$PID_FILE"))"
    else
        echo "$APP_NAME is not running"
    fi
}

logs() {
    if [ ! -f "$LOG_FILE" ]; then
        echo "No log file found"
        return 1
    fi
    tail -f "$LOG_FILE"
}

is_running() {
    [ -f "$PID_FILE" ] && kill -0 "$(cat "$PID_FILE")" 2>/dev/null
}

case "${1:-}" in
    start)   start ;;
    stop)    stop ;;
    restart) restart ;;
    rebuild) rebuild ;;
    build)   build ;;
    status)  status ;;
    logs)    logs ;;
    *)
        echo "Usage: $0 {start|stop|restart|rebuild|build|status|logs}"
        echo ""
        echo "  start    Start the bot"
        echo "  stop     Stop the bot (graceful, 10s timeout)"
        echo "  restart  Stop then start"
        echo "  rebuild  Stop, build, then start"
        echo "  build    Build binary only"
        echo "  status   Check if running"
        echo "  logs     Tail the log file"
        exit 1
        ;;
esac

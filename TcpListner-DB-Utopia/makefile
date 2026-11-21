APP_NAME := db-utopia-tcp-app
MAIN_PATH := ./cmd/main.go

BUILD_DIR := build

# -----------------------------
# Build main app (Linux)
# -----------------------------
linux:
	@echo "🐧 Building main app for Linux..."
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(APP_NAME) $(MAIN_PATH)
	@echo "✅ Main app: $(BUILD_DIR)/$(APP_NAME)"


# -----------------------------
# Windows build (optional)
# -----------------------------
build:
	@echo "🧱 Building Windows binaries..."
	@mkdir -p $(BUILD_DIR)
	go build -o $(BUILD_DIR)/$(APP_NAME).exe $(MAIN_PATH)
	@echo "✅ Windows builds complete"

# -----------------------------
# Clean
# -----------------------------
clean:
	@echo "🧹 Cleaning build directory..."
	rm -rf $(BUILD_DIR)
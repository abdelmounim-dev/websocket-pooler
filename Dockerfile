# /home/ab/ws-pooler/Dockerfile

# ---- Build Stage ----
# Use an official Go image as the builder.
# Using alpine for a smaller base.
FROM golang:1.24-alpine AS builder

# Set the working directory inside the container
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker's build cache.
# This step is only re-run if these files change.
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the application, creating a static binary.
# CGO_ENABLED=0 is important for creating a static binary that works in a minimal container.
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/pooler .

# ---- Final Stage ----
# Use a minimal base image for the final container.
FROM alpine:latest

# Set the working directory
WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /app/pooler /app/pooler

# Copy the configuration file. The app looks for it in the current directory.
COPY config.dev.yaml .

# Expose the ports the application listens on
EXPOSE 8080
EXPOSE 9090

# Set the command to run when the container starts
CMD ["/app/pooler"]

FROM golang:latest as builder
WORKDIR /app
COPY . .
RUN GOOS=linux CGO_ENABLED=0 go build -ldflags="-w -s" -o server ./main.go

FROM scratch 
COPY --from=builder /app/server .
ENV PORT="8080"
ENV DB_URL="postgresql://rinha:rinha@db:5432/rinha?sslmode=disable"
ENTRYPOINT ["./server"]

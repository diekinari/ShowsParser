FROM golang:1.24.1-alpine

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download && go mod verify

COPY . .

RUN go build -o showsparser .

CMD ["./main"]
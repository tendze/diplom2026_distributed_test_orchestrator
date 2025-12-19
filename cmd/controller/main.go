package main

import (
	"context"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"google.golang.org/grpc"

	pb "github.com/tendze/diplom2026_distributed_test_orchestrator/gen/controller"
	controllerservice "github.com/tendze/diplom2026_distributed_test_orchestrator/internal/controller"
)

func main() {
	lis, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatal(err)
	}

	s := grpc.NewServer()
	pb.RegisterControllerServiceServer(s, &controllerservice.ControllerService{})

	log.Println("Controller started on :9000")

	go controllerservice.StartTestOnAgent("localhost:9001")
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	
	log.Println("shutting down controller server")
	s.GracefulStop()
}

package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(time.Second*2))
	defer cancel()
	//go func(ctx context.Context) {
	//	time.Sleep(time.Second)
	//}(ctx)
	//time.Sleep(time.Second)

	select {
	case <-ctx.Done():
		fmt.Println("call successfully!!!")
		return
	case <-time.After(time.Duration(time.Millisecond * 900)):
		fmt.Println("timeout!!!")
		return
	}
}

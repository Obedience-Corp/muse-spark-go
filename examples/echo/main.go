// Command echo demonstrates a local, credential-free MSP conversation.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/Obedience-Corp/muse-spark-go/pkg/muse"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := muse.NewClientFromPath()
	if err != nil {
		log.Fatal(err)
	}
	run, err := c.RunPromptCtx(ctx, "hello", &muse.RunOptions{Provider: "echo", Preset: "empty", NoSessionLog: true, NoForeignPersonalContext: true, DisableRequestUserInput: true})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("exec:", run.Text)
	durable := os.Getenv("MUSE_TEST_DURABLE") == "1"
	h, err := c.StartMSP(ctx, muse.HostOptions{Provider: "echo", NoSessionLog: !durable})
	if err != nil {
		log.Fatal(err)
	}
	defer h.Close(context.Background())
	go func() {
		for n := range h.Notifications() {
			if n.Method == "item/delta" {
				fmt.Println(string(n.Params))
			}
		}
	}()
	dir := os.Getenv("MUSE_TEST_WORKSPACE")
	if dir == "" {
		dir, err = os.Getwd()
		if err != nil {
			log.Fatal(err)
		}
	}
	session, err := h.StartSession(ctx, muse.SessionOptions{WorkspaceRoot: dir})
	if err != nil {
		log.Fatal(err)
	}
	turn, err := session.Send(ctx, []muse.InputPart{muse.Text("hello")})
	if err != nil {
		log.Fatal(err)
	}
	result, err := turn.Wait(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("terminal:", result.Terminal)
	if durable {
		if err := h.Close(ctx); err != nil {
			log.Fatal(err)
		}
		h2, err := c.StartMSP(ctx, muse.HostOptions{Provider: "echo"})
		if err != nil {
			log.Fatal(err)
		}
		defer h2.Close(context.Background())
		go func() {
			for range h2.Notifications() {
			}
		}()
		resumed, _, err := h2.ResumeSession(ctx, session.Info.SessionID)
		if err != nil {
			log.Fatal(err)
		}
		if resumed.Info.SessionID != session.Info.SessionID {
			log.Fatal("resumed a different session")
		}
		next, err := resumed.Send(ctx, []muse.InputPart{muse.Text("again")})
		if err != nil {
			log.Fatal(err)
		}
		final, err := next.Wait(ctx)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("resumed terminal:", final.Terminal)
	}
}

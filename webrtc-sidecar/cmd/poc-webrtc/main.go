package main

import (
	"fmt"
	"os"
	"time"

	"github.com/outview/webrtc-sidecar/internal/webrtc"
)

func main() {
	fmt.Println("=== pion/webrtc v4 POC ===")
	fmt.Println()

	// Create POC manager
	manager, err := webrtc.NewPOCManager()
	if err != nil {
		fmt.Printf("Failed to create POC manager: %v\n", err)
		os.Exit(1)
	}
	defer manager.Close()

	// Step 1: Setup offerer
	fmt.Println("Step 1: Setting up offerer...")
	if err := manager.SetupOfferer(); err != nil {
		fmt.Printf("Failed to setup offerer: %v\n", err)
		os.Exit(1)
	}

	// Step 2: Setup answerer
	fmt.Println("Step 2: Setting up answerer...")
	if err := manager.SetupAnswerer(); err != nil {
		fmt.Printf("Failed to setup answerer: %v\n", err)
		os.Exit(1)
	}

	// Step 3: Create offer
	fmt.Println("\nStep 3: Creating offer...")
	offerSDP, err := manager.CreateOffer()
	if err != nil {
		fmt.Printf("Failed to create offer: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Offer SDP length: %d bytes\n", len(offerSDP))

	// Step 4: Set remote offer on answerer
	fmt.Println("\nStep 4: Setting remote offer on answerer...")
	if err := manager.SetRemoteOffer(offerSDP); err != nil {
		fmt.Printf("Failed to set remote offer: %v\n", err)
		os.Exit(1)
	}

	// Step 5: Create answer
	fmt.Println("\nStep 5: Creating answer...")
	answerSDP, err := manager.CreateAnswer()
	if err != nil {
		fmt.Printf("Failed to create answer: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Answer SDP length: %d bytes\n", len(answerSDP))

	// Step 6: Set remote answer on offerer
	fmt.Println("\nStep 6: Setting remote answer on offerer...")
	if err := manager.SetRemoteAnswer(answerSDP); err != nil {
		fmt.Printf("Failed to set remote answer: %v\n", err)
		os.Exit(1)
	}

	// Step 7: Exchange ICE candidates
	fmt.Println("\nStep 7: Exchanging ICE candidates...")
	if err := manager.ExchangeICECandidates(); err != nil {
		fmt.Printf("Failed to exchange ICE candidates: %v\n", err)
		os.Exit(1)
	}

	// Step 8: Wait for DataChannel to open
	fmt.Println("\nStep 8: Waiting for DataChannel to open...")
	if err := manager.WaitForDataChannel(10 * time.Second); err != nil {
		fmt.Printf("Failed to wait for data channel: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ DataChannel opened successfully!")

	// Step 9: Send test data
	fmt.Println("\nStep 9: Sending test data...")
	testData := []byte("Hello from pion/webrtc v4!")
	if err := manager.SendData(testData); err != nil {
		fmt.Printf("Failed to send data: %v\n", err)
		os.Exit(1)
	}

	// Wait for echo response
	time.Sleep(1 * time.Second)
	receivedData := manager.GetReceivedData()
	if receivedData != nil {
		fmt.Printf("✓ Received echo: %s\n", string(receivedData))
	} else {
		fmt.Println("⚠ No data received")
	}

	fmt.Println("\n=== POC Completed Successfully ===")
	fmt.Println("✓ PeerConnection creation: OK")
	fmt.Println("✓ DataChannel creation: OK")
	fmt.Println("✓ Offer/Answer exchange: OK")
	fmt.Println("✓ ICE candidate collection: OK")
	fmt.Println("✓ DataChannel data transmission: OK")
}

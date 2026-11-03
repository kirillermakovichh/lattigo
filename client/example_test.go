package client_test

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"example.com/go-block-lattice/client"
)

func ExampleClient_Batch() {
	account := client.Address(ed25519.NewKeyFromSeed(make([]byte, 32)).Public().(ed25519.PublicKey))
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		value := client.AccountState{Account: account, Representative: account, Balance: 25}
		json.NewEncoder(out).Encode([]map[string]any{{"id": "balance", "data": value}})
	}))
	defer server.Close()
	c, err := client.NewWithOptions(server.URL, "", client.WithTimeout(time.Second))
	if err != nil {
		panic(err)
	}
	defer c.CloseIdleConnections()
	rows, err := c.Batch(context.Background(), []client.ReadQuery{{ID: "balance", Operation: client.ReadAccount, Address: account}})
	if err != nil {
		panic(err)
	}
	var state client.AccountState
	if err := rows[0].Decode(&state); err != nil {
		panic(err)
	}
	fmt.Println(state.Balance.String())
	// Output: 25
}

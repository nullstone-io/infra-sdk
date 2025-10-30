package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	infra_sdk "github.com/nullstone-io/infra-sdk"
	"github.com/nullstone-io/infra-sdk/examples/aws/bootstrap"
)

// Month-to-date, daily, filtered to a single stack, grouped by env
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer cancel()

	// Inputs
	stacks := []string{"core"}
	end := time.Now()
	start := time.Date(end.Year(), end.Month(), 1, 0, 0, 0, 0, end.Location())

	coster, err := bootstrap.NewCoster()
	if err != nil {
		log.Fatalln(err.Error())
	}

	query := infra_sdk.CostQuery{
		Start:       start,
		End:         end,
		Granularity: infra_sdk.CostGranularityDaily,
		FilterTags: []infra_sdk.CostFilterTag{
			{
				Key:    infra_sdk.StandardTagStack,
				Values: stacks,
			},
		},
		GroupTags: []infra_sdk.CostGroupTag{{Key: infra_sdk.StandardTagEnv}},
	}
	result, err := coster.GetCosts(ctx, query)
	if err != nil {
		log.Fatalln(err.Error())
	}
	raw, _ := json.MarshalIndent(result, "", "  ")
	fmt.Println(string(raw))
}

package main

import (
	"context"
	"github.com/vitoordaz/robinhood-export/internal/robinhood"
	"sync"
)

type (
	listLoadFunc func(ctx context.Context, cursor string) (interface{}, string, error)
	itemLoadFunc func(ctx context.Context, id string) (interface{}, error)
	getIdFunc    func(interface{}) string
)

// loadList iterates over results of a given load function until cursor will not become empty.
func loadList(ctx context.Context, loadFunc listLoadFunc) (interface{}, error) {
	var result []interface{}
	cursor := ""
	for {
		items, cursor, err := loadFunc(ctx, cursor)
		if err != nil {
			return nil, err
		}
		result = append(result, items.([]interface{})...)
		if cursor == "" {
			break
		}
	}
	return result, nil
}

const maxConcurrency = 10

// loadDetails concurrently loads items with a given ids using given load function.
func loadDetails(ctx context.Context, ids []string, loadFunc itemLoadFunc) ([]interface{}, error) {
	type result struct {
		item interface{}
		err  error
	}

	ctx, cancel := context.WithCancel(ctx)
	resultsCh := make(chan *result)
	go func() {
		wg := sync.WaitGroup{}
		defer func() {
			wg.Wait() // wait for all requests to complete before closing resultsCh
			close(resultsCh)
		}()
		sem := make(chan interface{}, maxConcurrency)
		for _, i := range ids {
			wg.Add(1)
			select {
			case sem <- nil:
			case <-ctx.Done():
				return
			}
			go func(id string) {
				item, err := loadFunc(ctx, id)
				resultsCh <- &result{item, err}
				wg.Done()
				<-sem
			}(i)
		}
	}()

	items := make([]interface{}, 0, len(ids))
	for result := range resultsCh {
		if result.err != nil {
			// in case of error cancel pending requests and drain resultsCh
			cancel()
			for range resultsCh {
			}
			return nil, result.err
		}
		items = append(items, result.item)
	}
	return items, nil
}

func getIds(items interface{}, idFunc getIdFunc) []string {
	set := map[string]bool{}
	var ids []string
	for _, item := range items.([]interface{}) {
		id := idFunc(item)
		if !set[id] {
			ids = append(ids, id)
			set[id] = true
		}
	}
	return ids
}

func getInstrumentsMarketIds(instruments []*robinhood.Instrument) []string {
	return getIds(instruments, func(instrument interface{}) string {
		return instrument.(*robinhood.Instrument).Market
	})
}

func loadInstruments(ctx context.Context, client robinhood.Client, ids []string) ([]*robinhood.Instrument, error) {
	items, err := loadDetails(ctx, ids, func(ctx context.Context, id string) (interface{}, error) {
		return client.GetInstrument(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	instruments := make([]*robinhood.Instrument, 0, len(items))
	for _, item := range items {
		instruments = append(instruments, item.(*robinhood.Instrument))
	}
	return instruments, nil
}

func loadMarkets(ctx context.Context, client robinhood.Client, ids []string) ([]*robinhood.Market, error) {
	items, err := loadDetails(ctx, ids, func(ctx context.Context, id string) (interface{}, error) {
		return client.GetMarket(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	markets := make([]*robinhood.Market, 0, len(items))
	for _, item := range items {
		markets = append(markets, item.(*robinhood.Market))
	}
	return markets, nil
}

func getInstrumentByURL(instruments []*robinhood.Instrument) map[string]*robinhood.Instrument {
	instrumentByURL := make(map[string]*robinhood.Instrument, len(instruments))
	for _, instrument := range instruments {
		instrumentByURL[instrument.URL] = instrument
	}
	return instrumentByURL
}

func getMarketByURL(markets []*robinhood.Market) map[string]*robinhood.Market {
	marketByURL := make(map[string]*robinhood.Market, len(markets))
	for _, market := range markets {
		marketByURL[market.URL] = market
	}
	return marketByURL
}
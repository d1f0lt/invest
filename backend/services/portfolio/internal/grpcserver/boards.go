package grpcserver

import (
	"context"
	"time"

	"invest/backend/services/portfolio/internal/pnl"
	"invest/backend/services/portfolio/internal/storage"
)

const staleBoardAfter = 72 * time.Hour

func pickPriceBoard(board string, seen []storage.BoardPrice) string {
	var own, fresh *storage.BoardPrice
	for i := range seen {
		p := &seen[i]
		if p.Board == board {
			own = p
		}
		if fresh == nil || p.CollectedAt.After(fresh.CollectedAt) {
			fresh = p
		}
	}
	if fresh == nil || fresh.Board == board {
		return board
	}
	if own != nil && fresh.CollectedAt.Sub(own.CollectedAt) <= staleBoardAfter {
		return board
	}
	return fresh.Board
}

func (s *Server) resolvePriceBoards(ctx context.Context, b *book) error {
	if len(b.instruments) == 0 {
		return nil
	}
	secids := make([]string, 0, len(b.instruments))
	seenSec := map[string]bool{}
	for _, inst := range b.instruments {
		if !seenSec[inst[0]] {
			seenSec[inst[0]] = true
			secids = append(secids, inst[0])
		}
	}
	boards, err := s.Store.PriceBoards(ctx, secids)
	if err != nil {
		return err
	}

	remap := map[string]string{}
	var instruments [][2]string
	added := map[[2]string]bool{}
	for _, inst := range b.instruments {
		target := [2]string{inst[0], pickPriceBoard(inst[1], boards[inst[0]])}
		if target != inst {
			remap[pnl.PriceKey(inst[0], inst[1])] = target[1]
			if b.aliases == nil {
				b.aliases = map[[2]string][][2]string{}
			}
			b.aliases[target] = append(b.aliases[target], inst)
		}
		if !added[target] {
			added[target] = true
			instruments = append(instruments, target)
		}
	}
	if len(remap) == 0 {
		return nil
	}
	b.instruments = instruments

	for li := range b.ledgers {
		l := &b.ledgers[li]
		for i := range l.trades {
			if nb, ok := remap[pnl.PriceKey(l.trades[i].SecID, l.trades[i].Board)]; ok {
				l.trades[i].Board = nb
			}
		}
		for i := range l.cash {
			if l.cash[i].SecID == "" {
				continue
			}
			if nb, ok := remap[pnl.PriceKey(l.cash[i].SecID, l.cash[i].Board)]; ok {
				l.cash[i].Board = nb
			}
		}
	}
	return nil
}

func mergeAliasCloses(own, alias []storage.DailyClose) []storage.DailyClose {
	if len(alias) == 0 {
		return own
	}
	var cutoff time.Time
	if len(own) > 0 {
		cutoff = own[0].Day
	}
	out := make([]storage.DailyClose, 0, len(alias)+len(own))
	for _, c := range alias {
		if cutoff.IsZero() || c.Day.Before(cutoff) {
			out = append(out, c)
		}
	}
	return append(out, own...)
}

package grpcserver

import (
	"context"
	"strconv"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"invest/backend/services/securities_reader/internal/moex"
	securitiesreaderpb "invest/backend/services/securities_reader/proto"
)

type infoSpec struct {
	moexName string
	name     string
	title    string
	kind     string
	unit     string
	unitFrom string
}

var infoSpecs = []infoSpec{
	{moexName: "NAME", name: "NAME", title: "Название", kind: "text"},
	{moexName: "TYPENAME", name: "TYPE", title: "Тип бумаги", kind: "text"},
	{moexName: "ISIN", name: "ISIN", title: "ISIN", kind: "text"},
	{moexName: "REGNUMBER", name: "REGNUMBER", title: "Рег. номер", kind: "text"},
	{name: "LOTSIZE", title: "Лот", kind: "number", unit: "шт."},
	{moexName: "FACEVALUE", name: "FACEVALUE", title: "Номинал", kind: "money", unitFrom: "FACEUNIT"},
	{moexName: "ISSUESIZE", name: "ISSUESIZE", title: "Объём выпуска", kind: "number", unit: "шт."},
	{moexName: "ISSUEDATE", name: "ISSUEDATE", title: "Начало торгов", kind: "date"},
	{moexName: "LISTLEVEL", name: "LISTLEVEL", title: "Уровень листинга", kind: "number"},
	{moexName: "MATDATE", name: "MATDATE", title: "Дата погашения", kind: "date"},
	{moexName: "OFFERDATE", name: "OFFERDATE", title: "Дата оферты", kind: "date"},
	{moexName: "COUPONPERCENT", name: "COUPONPERCENT", title: "Ставка купона", kind: "percent"},
	{moexName: "COUPONVALUE", name: "COUPONVALUE", title: "Размер купона", kind: "money", unitFrom: "FACEUNIT"},
	{moexName: "COUPONFREQUENCY", name: "COUPONFREQUENCY", title: "Выплат купона в год", kind: "number"},
	{moexName: "ISQUALIFIEDINVESTORS", name: "QUALIFIED", title: "Для квал. инвесторов", kind: "bool"},
}

func (s *Server) GetSecurityInfo(ctx context.Context, req *securitiesreaderpb.GetSecurityInfoRequest) (*securitiesreaderpb.GetSecurityInfoResponse, error) {
	secid := strings.ToUpper(strings.TrimSpace(req.GetSecid()))
	if secid == "" {
		return nil, status.Error(codes.InvalidArgument, "secid is required")
	}
	board, err := s.resolveBoard(ctx, secid, strings.ToUpper(strings.TrimSpace(req.GetBoard())))
	if err != nil {
		return nil, err
	}

	s.caches()
	key := secid + "|" + board
	if cached, ok := s.info.get(key, s.now()); ok {
		return cached, nil
	}

	ref, _, err := s.Store.SecurityRef(ctx, secid, board)
	if err != nil {
		s.Log.Error("failed to load security", "secid", secid, "board", board, "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}

	values := map[string]string{}
	if ref.SecName != nil {
		values["NAME"] = *ref.SecName
	}
	if ref.ISIN != nil {
		values["ISIN"] = *ref.ISIN
	}
	if ref.LotSize != nil && *ref.LotSize > 0 {
		values["LOTSIZE"] = strconv.FormatInt(*ref.LotSize, 10)
	}

	var issuer string
	complete := s.Moex != nil
	if s.Moex != nil {
		desc, err := s.Moex.Description(ctx, secid)
		if err != nil {
			complete = false
			s.Log.Warn("moex description unavailable", "secid", secid, "error", err)
		}
		for _, f := range desc {
			if f.Value != "" {
				values[strings.ToUpper(f.Name)] = f.Value
			}
		}
		issuer, err = s.Moex.EmitterTitle(ctx, secid)
		if err != nil {
			complete = false
			s.Log.Warn("moex issuer unavailable", "secid", secid, "error", err)
		}
	}

	resp := &securitiesreaderpb.GetSecurityInfoResponse{Secid: secid, Board: board}
	if issuer = shortIssuerName(issuer); issuer != "" {
		resp.Fields = append(resp.Fields, &securitiesreaderpb.SecurityInfoField{
			Name: "ISSUER", Title: "Эмитент", Value: issuer, Type: "text",
		})
	}
	resp.Fields = append(resp.Fields, buildInfoFields(values, moex.MarketFor(board) == "bonds")...)

	if complete {
		s.info.put(key, resp, s.now())
	}
	return resp, nil
}

func buildInfoFields(values map[string]string, bond bool) []*securitiesreaderpb.SecurityInfoField {
	var out []*securitiesreaderpb.SecurityInfoField
	for _, spec := range infoSpecs {
		src := spec.moexName
		if src == "" {
			src = spec.name
		}
		v := strings.TrimSpace(values[src])
		if v == "" {
			continue
		}
		if spec.name == "FACEVALUE" && !bond {
			continue
		}
		if spec.name == "QUALIFIED" && v != "1" {
			continue
		}
		if spec.kind == "number" || spec.kind == "money" || spec.kind == "percent" {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				continue
			}
		}
		f := &securitiesreaderpb.SecurityInfoField{Name: spec.name, Title: spec.title, Value: v, Type: spec.kind}
		switch {
		case spec.unit != "":
			u := spec.unit
			f.Unit = &u
		case spec.unitFrom != "":
			if u := strings.TrimSpace(values[spec.unitFrom]); u != "" {
				f.Unit = &u
			}
		}
		out = append(out, f)
	}
	return out
}

var legalForms = []struct{ long, short string }{
	{"международная компания публичное акционерное общество", "МКПАО"},
	{"публичное акционерное общество", "ПАО"},
	{"непубличное акционерное общество", "АО"},
	{"открытое акционерное общество", "ОАО"},
	{"закрытое акционерное общество", "ЗАО"},
	{"акционерное общество", "АО"},
	{"общество с ограниченной ответственностью", "ООО"},
}

func shortIssuerName(name string) string {
	name = strings.Join(strings.Fields(name), " ")
	lower := strings.ToLower(name)
	for _, f := range legalForms {
		if strings.HasPrefix(lower, f.long) {
			name = f.short + name[len(f.long):]
			break
		}
	}
	if strings.Count(name, `"`)%2 == 0 {
		var b strings.Builder
		open := true
		for _, r := range name {
			if r == '"' {
				if open {
					b.WriteRune('«')
				} else {
					b.WriteRune('»')
				}
				open = !open
				continue
			}
			b.WriteRune(r)
		}
		name = b.String()
	}
	return name
}

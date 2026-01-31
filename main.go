package main

import (
	"fmt"
	"image/color"
	"math"
	"sort"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type softTheme struct{}

func (s softTheme) Color(n fyne.ThemeColorName, v fyne.ThemeVariant) color.Color {
	switch n {
	case theme.ColorNameBackground:
		return color.NRGBA{R: 250, G: 250, B: 252, A: 255}
	case theme.ColorNameButton:
		return color.NRGBA{R: 230, G: 235, B: 245, A: 255}
	case theme.ColorNamePrimary:
		return color.NRGBA{R: 70, G: 130, B: 180, A: 255}
	case theme.ColorNameForeground:
		return color.NRGBA{R: 40, G: 40, B: 50, A: 255}
	case theme.ColorNameInputBackground:
		return color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	case theme.ColorNamePlaceHolder:
		return color.NRGBA{R: 140, G: 140, B: 150, A: 255}
	}
	return theme.DefaultTheme().Color(n, theme.VariantLight)
}

func (s softTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (s softTheme) Icon(n fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(n)
}

func (s softTheme) Size(n fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(n)
}

// Narrow entry wrapper
type narrowEntry struct {
	widget.Entry
}

func newNarrowEntry() *narrowEntry {
	e := &narrowEntry{}
	e.ExtendBaseWidget(e)
	return e
}

func (e *narrowEntry) MinSize() fyne.Size {
	return fyne.NewSize(50, e.Entry.MinSize().Height)
}

// ============ STORED RESULTS FOR PEDAGOGY ============

var lastFTRResult struct {
	Solved                                      bool
	Node1LMP, Node2LMP, Flow, RentPool, ProRata float64
	Congested                                   bool
	Traders                                     []Trader
	FTRs                                        []FTRBid
	Virtuals                                    []VirtualBid
	RTLMP1, RTLMP2                              float64
}

var lastDARTResult struct {
	Solved             bool
	DALMP, RTLMP       float64
	Traders            []Trader
	Virtuals           []VirtBid
	DADemand, RTDemand float64
}

// ============ SHARED TYPES ============

type Trader struct {
	Name       string
	GenProfit  float64
	VirtProfit float64
	FTRProfit  float64
	Total      float64
}

type Generator struct {
	Name     string
	MW       float64
	Cost     float64
	OwnerIdx int // index into players list, -1 = unassigned
	Node     int // 1 or 2 for two-node games
}

// Global shared data for Teacher tab
var players = []string{"SD", "RK", "KM", "RM", "RS", "NK", "MK", "AF", "OT", "SF", "VK", "MPC"}
var generators = []*Generator{
	// Node 1: $5 cost generators
	{"SD-1", 16, 5, 0, 1},
	{"RK-1", 15, 5, 1, 1},
	{"KM-1", 16, 5, 2, 1},
	{"RS-1", 15, 5, 4, 1},
	{"NK-1", 18, 5, 5, 1},
	{"OT-1", 15, 5, 8, 1},
	// Node 2: $25 cost generators
	{"SF-1", 20, 25, 9, 2},
	{"MPC-1", 15, 25, 11, 2},
	{"MK-1", 15, 25, 6, 2},
	{"AF-1", 20, 25, 7, 2},
	{"VK-1", 15, 25, 10, 2},
	{"RM-1", 15, 25, 3, 2},
}

type Bid struct {
	TraderIdx int
	MW        float64
	Price     float64
	Node      int
	Virtual   bool
	Dispatch  float64
	Profit    float64
}

type VirtualBid struct {
	TraderIdx int
	MW        float64
	Price     float64
	Node      int
	Type      string
	Cleared   float64
	NetPnL    float64
}

type FTRBid struct {
	TraderIdx int
	MW        float64
	BidPrice  float64
	Entitle   float64
	Payment   float64
	Cost      float64
	Profit    float64
}

// ============ TAB 1: TWO-NODE FTR ============

func solveTwoNode(traders []Trader, bids []Bid, virtuals []VirtualBid, demand1, demand2, cableCap, rtLMP1, rtLMP2 float64, ftrs []FTRBid) (sol struct {
	Node1LMP, Node2LMP, Flow, RentPool, ProRata float64
	Congested                                   bool
	Bids                                        []Bid
	Virtuals                                    []VirtualBid
	FTRs                                        []FTRBid
	Traders                                     []Trader
}) {
	for i := range traders {
		traders[i].GenProfit, traders[i].VirtProfit, traders[i].FTRProfit, traders[i].Total = 0, 0, 0, 0
	}

	supply := make([]Bid, len(bids))
	copy(supply, bids)
	for _, v := range virtuals {
		if v.Type == "INC" {
			supply = append(supply, Bid{TraderIdx: v.TraderIdx, MW: v.MW, Price: v.Price, Node: v.Node, Virtual: true})
		}
	}
	sort.Slice(supply, func(i, j int) bool { return supply[i].Price < supply[j].Price })

	decDemand1, decDemand2 := 0.0, 0.0
	for _, v := range virtuals {
		if v.Type == "DEC" {
			if v.Node == 1 {
				decDemand1 += v.MW
			} else {
				decDemand2 += v.MW
			}
		}
	}
	totalDemand := demand1 + demand2 + decDemand1 + decDemand2

	var cumMW, uncongPrice float64
	for i := range supply {
		cumMW += supply[i].MW
		if cumMW >= totalDemand {
			uncongPrice = supply[i].Price
			break
		}
	}

	cumMW = 0
	node1Gen, node2Gen := 0.0, 0.0
	for i := range supply {
		if cumMW >= totalDemand {
			supply[i].Dispatch = 0
		} else if cumMW+supply[i].MW <= totalDemand {
			supply[i].Dispatch = supply[i].MW
		} else {
			supply[i].Dispatch = totalDemand - cumMW
		}
		cumMW += supply[i].MW
		if supply[i].Node == 1 {
			node1Gen += supply[i].Dispatch
		} else {
			node2Gen += supply[i].Dispatch
		}
	}

	adjustedDemand1 := demand1 + decDemand1
	impliedFlow := node1Gen - adjustedDemand1

	if math.Abs(impliedFlow) <= cableCap {
		sol.Congested = false
		sol.Node1LMP, sol.Node2LMP = uncongPrice, uncongPrice
		sol.Flow = impliedFlow
		sol.Bids = supply
	} else {
		sol.Congested = true
		if impliedFlow > 0 {
			sol.Flow = cableCap
		} else {
			sol.Flow = -cableCap
		}
		n1NetDemand := adjustedDemand1 - sol.Flow
		n2NetDemand := (demand2 + decDemand2) + sol.Flow

		var n1, n2 []Bid
		for _, b := range supply {
			if b.Node == 1 {
				n1 = append(n1, b)
			} else {
				n2 = append(n2, b)
			}
		}

		cumMW = 0
		for i := range n1 {
			if cumMW >= n1NetDemand {
				n1[i].Dispatch = 0
			} else if cumMW+n1[i].MW <= n1NetDemand {
				n1[i].Dispatch = n1[i].MW
			} else {
				n1[i].Dispatch = n1NetDemand - cumMW
			}
			if n1[i].Dispatch > 0 {
				sol.Node1LMP = n1[i].Price
			}
			cumMW += n1[i].MW
		}
		cumMW = 0
		for i := range n2 {
			if cumMW >= n2NetDemand {
				n2[i].Dispatch = 0
			} else if cumMW+n2[i].MW <= n2NetDemand {
				n2[i].Dispatch = n2[i].MW
			} else {
				n2[i].Dispatch = n2NetDemand - cumMW
			}
			if n2[i].Dispatch > 0 {
				sol.Node2LMP = n2[i].Price
			}
			cumMW += n2[i].MW
		}
		sol.Bids = append(n1, n2...)
	}

	for i := range sol.Bids {
		if sol.Bids[i].Virtual {
			continue
		}
		lmp := sol.Node1LMP
		if sol.Bids[i].Node == 2 {
			lmp = sol.Node2LMP
		}
		sol.Bids[i].Profit = (lmp - sol.Bids[i].Price) * sol.Bids[i].Dispatch
		if idx := sol.Bids[i].TraderIdx; idx >= 0 && idx < len(traders) {
			traders[idx].GenProfit += sol.Bids[i].Profit
		}
	}

	for i := range virtuals {
		daLMP, rtLMP := sol.Node1LMP, rtLMP1
		if virtuals[i].Node == 2 {
			daLMP, rtLMP = sol.Node2LMP, rtLMP2
		}
		virtuals[i].Cleared = 0
		if virtuals[i].Type == "INC" && virtuals[i].Price <= daLMP {
			virtuals[i].Cleared = virtuals[i].MW
		} else if virtuals[i].Type == "DEC" && virtuals[i].Price >= daLMP {
			virtuals[i].Cleared = virtuals[i].MW
		}
		if virtuals[i].Type == "INC" {
			virtuals[i].NetPnL = (daLMP - rtLMP) * virtuals[i].Cleared
		} else {
			virtuals[i].NetPnL = (rtLMP - daLMP) * virtuals[i].Cleared
		}
		if idx := virtuals[i].TraderIdx; idx >= 0 && idx < len(traders) {
			traders[idx].VirtProfit += virtuals[i].NetPnL
		}
	}
	sol.Virtuals = virtuals

	sol.RentPool = math.Abs(sol.Flow) * math.Abs(sol.Node2LMP-sol.Node1LMP)
	lmpDiff := sol.Node1LMP - sol.Node2LMP
	totalEntitle := 0.0
	for i := range ftrs {
		ftrs[i].Entitle = ftrs[i].MW * math.Max(0, lmpDiff)
		totalEntitle += ftrs[i].Entitle
	}
	if totalEntitle > 0 {
		sol.ProRata = math.Min(1.0, sol.RentPool/totalEntitle)
	}
	for i := range ftrs {
		ftrs[i].Payment = ftrs[i].Entitle * sol.ProRata
		ftrs[i].Cost = ftrs[i].MW * ftrs[i].BidPrice
		ftrs[i].Profit = ftrs[i].Payment - ftrs[i].Cost
		if idx := ftrs[i].TraderIdx; idx >= 0 && idx < len(traders) {
			traders[idx].FTRProfit += ftrs[i].Profit
		}
	}
	sol.FTRs = ftrs

	for i := range traders {
		traders[i].Total = traders[i].GenProfit + traders[i].VirtProfit + traders[i].FTRProfit
	}
	sol.Traders = traders
	return
}

// ============ TAB 2: SINGLE-NODE DA/RT ============

type PhysBid struct {
	TraderIdx int
	MW        float64
	DAPrice   float64
	RTPrice   float64
	DADisp    float64
	RTDisp    float64
	Profit    float64
}

type VirtBid struct {
	TraderIdx int
	Type      string // INC, DEC, or ""
	MW        float64
	Price     float64
	Cleared   float64
	NetPnL    float64
}

func solveDART2(traders []Trader, phys []PhysBid, virts []VirtBid, daDemand, rtDemand, incCap, decCap float64) (daLMP, rtLMP float64, outPhys []PhysBid, outVirts []VirtBid, outTraders []Trader) {
	for i := range traders {
		traders[i].GenProfit, traders[i].VirtProfit, traders[i].FTRProfit, traders[i].Total = 0, 0, 0, 0
	}

	// ========== STEP 1: Determine which virtuals can participate (apply caps) ==========
	type virtEntry struct {
		idx   int
		mw    float64
		price float64
	}
	var incBids, decBids []virtEntry
	for i, v := range virts {
		if v.Type == "INC" && v.MW > 0 {
			incBids = append(incBids, virtEntry{i, v.MW, v.Price})
		} else if v.Type == "DEC" && v.MW > 0 {
			decBids = append(decBids, virtEntry{i, v.MW, v.Price})
		}
	}
	// INCs: lowest price first (most competitive supply)
	sort.Slice(incBids, func(a, b int) bool { return incBids[a].price < incBids[b].price })
	// DECs: highest price first (most aggressive demand)
	sort.Slice(decBids, func(a, b int) bool { return decBids[a].price > decBids[b].price })

	incParticipate := make(map[int]float64)
	decParticipate := make(map[int]float64)
	incUsed, decUsed := 0.0, 0.0
	for _, inc := range incBids {
		canUse := math.Min(inc.mw, incCap-incUsed)
		if canUse > 0 {
			incParticipate[inc.idx] = canUse
			incUsed += canUse
		}
	}
	for _, dec := range decBids {
		canUse := math.Min(dec.mw, decCap-decUsed)
		if canUse > 0 {
			decParticipate[dec.idx] = canUse
			decUsed += canUse
		}
	}

	// ========== STEP 2: DAY-AHEAD MARKET ==========
	// Supply: Physical generators (DA prices) + INC bids (virtual supply)
	// Demand: Fixed load + DEC bids (virtual demand)
	type supBid struct {
		mw, price float64
		physIdx   int
		virtIdx   int
	}
	var daSupply []supBid
	for i, p := range phys {
		if p.MW > 0 {
			daSupply = append(daSupply, supBid{p.MW, p.DAPrice, i, -1})
		}
	}
	for idx, mw := range incParticipate {
		daSupply = append(daSupply, supBid{mw, virts[idx].Price, -1, idx})
	}
	sort.Slice(daSupply, func(i, j int) bool { return daSupply[i].price < daSupply[j].price })

	daTotalDemand := daDemand + decUsed

	// Clear DA market (dispatch lowest cost first)
	physDADisp := make(map[int]float64)
	incCleared := make(map[int]float64)
	remaining := daTotalDemand
	for _, s := range daSupply {
		if remaining <= 0 {
			break
		}
		disp := math.Min(s.mw, remaining)
		if disp > 0 {
			daLMP = s.price
			if s.physIdx >= 0 {
				physDADisp[s.physIdx] = disp
			} else {
				incCleared[s.virtIdx] = disp
			}
			remaining -= disp
		}
	}

	// DECs that participated are considered cleared (they added to demand)
	decCleared := make(map[int]float64)
	for idx, mw := range decParticipate {
		decCleared[idx] = mw
	}

	// ========== STEP 3: REAL-TIME MARKET ==========
	// Only physical generators participate (no virtuals)
	var rtSupply []supBid
	for i, p := range phys {
		if p.MW > 0 {
			rtSupply = append(rtSupply, supBid{p.MW, p.RTPrice, i, -1})
		}
	}
	sort.Slice(rtSupply, func(i, j int) bool { return rtSupply[i].price < rtSupply[j].price })

	physRTDisp := make(map[int]float64)
	remaining = rtDemand
	for _, s := range rtSupply {
		if remaining <= 0 {
			break
		}
		disp := math.Min(s.mw, remaining)
		if disp > 0 {
			rtLMP = s.price
			physRTDisp[s.physIdx] = disp
			remaining -= disp
		}
	}

	// ========== STEP 4: SETTLEMENT ==========
	// Physical generators: paid RT LMP for RT dispatch, cost is their marginal cost
	outPhys = make([]PhysBid, len(phys))
	copy(outPhys, phys)
	for i := range outPhys {
		outPhys[i].DADisp = physDADisp[i]
		outPhys[i].RTDisp = physRTDisp[i]
		// Profit = (RT LMP - marginal cost) × RT dispatch
		outPhys[i].Profit = (rtLMP - outPhys[i].RTPrice) * outPhys[i].RTDisp
		if idx := outPhys[i].TraderIdx; idx >= 0 && idx < len(traders) {
			traders[idx].GenProfit += outPhys[i].Profit
		}
	}

	// Virtual settlement:
	// INC: Sold in DA at DA LMP, must buy back in RT at RT LMP
	//      Profit = (DA LMP - RT LMP) × cleared MW
	// DEC: Bought in DA at DA LMP, sell in RT at RT LMP
	//      Profit = (RT LMP - DA LMP) × cleared MW
	outVirts = make([]VirtBid, len(virts))
	copy(outVirts, virts)
	for i := range outVirts {
		if outVirts[i].Type == "INC" {
			outVirts[i].Cleared = incCleared[i]
			outVirts[i].NetPnL = (daLMP - rtLMP) * outVirts[i].Cleared
		} else if outVirts[i].Type == "DEC" {
			outVirts[i].Cleared = decCleared[i]
			outVirts[i].NetPnL = (rtLMP - daLMP) * outVirts[i].Cleared
		}
		if idx := outVirts[i].TraderIdx; idx >= 0 && idx < len(traders) {
			traders[idx].VirtProfit += outVirts[i].NetPnL
		}
	}

	for i := range traders {
		traders[i].Total = traders[i].GenProfit + traders[i].VirtProfit
	}
	outTraders = traders
	return
}

// ============ MAIN ============

func main() {
	a := app.New()
	a.Settings().SetTheme(&softTheme{})
	w := a.NewWindow("Bid Stack Solver")
	w.Resize(fyne.NewSize(1400, 900))

	// ===== TWO-NODE GAMES (shared data) =====
	// Build traders from shared players list
	buildTraders := func() []*Trader {
		t := make([]*Trader, len(players))
		for i, name := range players {
			t[i] = &Trader{name, 0, 0, 0, 0}
		}
		return t
	}
	tnTraders := buildTraders()

	// Build bids from generators
	buildBidsFromGens := func() []*Bid {
		var bids []*Bid
		for _, g := range generators {
			if g.OwnerIdx >= 0 {
				bids = append(bids, &Bid{g.OwnerIdx, g.MW, g.Cost, g.Node, false, 0, 0})
			}
		}
		return bids
	}
	tnBids := buildBidsFromGens()

	tnVirts := []*VirtualBid{
		{0, 20, 8, 1, "INC", 0, 0}, {1, 15, 12, 2, "INC", 0, 0},
		{2, 10, 20, 1, "DEC", 0, 0}, {3, 25, 18, 2, "DEC", 0, 0},
	}
	tnFTRs := []*FTRBid{
		{4, 10, 3, 0, 0, 0, 0}, {6, 10, 7, 0, 0, 0, 0}, {2, 20, 12, 0, 0, 0, 0},
		{9, 45, 13, 0, 0, 0, 0}, {2, 10, 18, 0, 0, 0, 0},
	}

	t1d1 := newNarrowEntry()
	t1d1.SetText("94")
	t1d2 := newNarrowEntry()
	t1d2.SetText("68")
	t1cable := newNarrowEntry()
	t1cable.SetText("50")
	t1rt1 := newNarrowEntry()
	t1rt1.SetText("12")
	t1rt2 := newNarrowEntry()
	t1rt2.SetText("15")
	t1result := widget.NewLabel("")
	t1result.Wrapping = fyne.TextWrapWord

	var t1Refresh func()
	var t1n1Box, t1n2Box, t1vBox, t1fBox, t1sBox *fyne.Container

	t1MakeBidForm := func(node int) *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(5,
			widget.NewLabel("Trader"), widget.NewLabel("MW"), widget.NewLabel("$"),
			widget.NewLabel("Disp"), widget.NewLabel("Profit"))}
		// Sort by price (merit order)
		sorted := make([]*Bid, 0)
		for _, b := range tnBids {
			if b.Node == node {
				sorted = append(sorted, b)
			}
		}
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Price < sorted[j].Price })
		for _, b := range sorted {
			bid := b
			sel := widget.NewSelect(func() []string {
				n := make([]string, len(tnTraders))
				for i, t := range tnTraders {
					n[i] = t.Name
				}
				return n
			}(), func(s string) {
				for i, t := range tnTraders {
					if t.Name == s {
						bid.TraderIdx = i
						break
					}
				}
			})
			sel.SetSelectedIndex(bid.TraderIdx)
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.1f", bid.MW))
			mw.OnChanged = func(s string) { bid.MW, _ = strconv.ParseFloat(s, 64) }
			pr := newNarrowEntry()
			pr.SetText(fmt.Sprintf("%.0f", bid.Price))
			pr.OnChanged = func(s string) { bid.Price, _ = strconv.ParseFloat(s, 64) }
			rows = append(rows, container.NewGridWithColumns(5, sel, mw, pr,
				widget.NewLabel(fmt.Sprintf("%.1f", bid.Dispatch)),
				widget.NewLabel(fmt.Sprintf("$%.0f", bid.Profit))))
		}
		return container.NewVBox(rows...)
	}

	t1MakeVirtForm := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(7,
			widget.NewLabel("Trader"), widget.NewLabel("Type"), widget.NewLabel("Node"),
			widget.NewLabel("MW"), widget.NewLabel("$"), widget.NewLabel("Clr"), widget.NewLabel("PnL"))}
		// Sort by price (merit order)
		sorted := make([]*VirtualBid, len(tnVirts))
		copy(sorted, tnVirts)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Price < sorted[j].Price })
		for _, v := range sorted {
			vb := v
			sel := widget.NewSelect(func() []string {
				n := make([]string, len(tnTraders))
				for i, t := range tnTraders {
					n[i] = t.Name
				}
				return n
			}(), func(s string) {
				for i, t := range tnTraders {
					if t.Name == s {
						vb.TraderIdx = i
						break
					}
				}
			})
			sel.SetSelectedIndex(vb.TraderIdx)
			ty := widget.NewSelect([]string{"INC", "DEC"}, func(s string) { vb.Type = s })
			ty.SetSelected(vb.Type)
			nd := widget.NewSelect([]string{"1", "2"}, func(s string) { vb.Node, _ = strconv.Atoi(s) })
			nd.SetSelected(fmt.Sprintf("%d", vb.Node))
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.0f", vb.MW))
			mw.OnChanged = func(s string) { vb.MW, _ = strconv.ParseFloat(s, 64) }
			pr := newNarrowEntry()
			pr.SetText(fmt.Sprintf("%.0f", vb.Price))
			pr.OnChanged = func(s string) { vb.Price, _ = strconv.ParseFloat(s, 64) }
			rows = append(rows, container.NewGridWithColumns(7, sel, ty, nd, mw, pr,
				widget.NewLabel(fmt.Sprintf("%.0f", vb.Cleared)),
				widget.NewLabel(fmt.Sprintf("$%.0f", vb.NetPnL))))
		}
		return container.NewVBox(rows...)
	}

	t1MakeFTRForm := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(6,
			widget.NewLabel("Trader"), widget.NewLabel("MW"), widget.NewLabel("Bid"),
			widget.NewLabel("Entitle"), widget.NewLabel("Pay"), widget.NewLabel("Profit"))}
		// Sort by bid price
		sorted := make([]*FTRBid, len(tnFTRs))
		copy(sorted, tnFTRs)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].BidPrice < sorted[j].BidPrice })
		for _, f := range sorted {
			ftr := f
			sel := widget.NewSelect(func() []string {
				n := make([]string, len(tnTraders))
				for i, t := range tnTraders {
					n[i] = t.Name
				}
				return n
			}(), func(s string) {
				for i, t := range tnTraders {
					if t.Name == s {
						ftr.TraderIdx = i
						break
					}
				}
			})
			sel.SetSelectedIndex(ftr.TraderIdx)
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.0f", ftr.MW))
			mw.OnChanged = func(s string) { ftr.MW, _ = strconv.ParseFloat(s, 64) }
			bd := newNarrowEntry()
			bd.SetText(fmt.Sprintf("%.0f", ftr.BidPrice))
			bd.OnChanged = func(s string) { ftr.BidPrice, _ = strconv.ParseFloat(s, 64) }
			rows = append(rows, container.NewGridWithColumns(6, sel, mw, bd,
				widget.NewLabel(fmt.Sprintf("$%.0f", ftr.Entitle)),
				widget.NewLabel(fmt.Sprintf("$%.0f", ftr.Payment)),
				widget.NewLabel(fmt.Sprintf("$%.0f", ftr.Profit))))
		}
		return container.NewVBox(rows...)
	}

	t1MakeSummary := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(5,
			widget.NewLabel("Trader"), widget.NewLabel("Gen"), widget.NewLabel("Virt"),
			widget.NewLabel("FTR"), widget.NewLabel("Total"))}
		for _, t := range tnTraders {
			rows = append(rows, container.NewGridWithColumns(5,
				widget.NewLabel(t.Name),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.GenProfit)),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.VirtProfit)),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.FTRProfit)),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.Total))))
		}
		return container.NewVBox(rows...)
	}

	t1Solve := widget.NewButton("Solve", func() {
		d1, _ := strconv.ParseFloat(t1d1.Text, 64)
		d2, _ := strconv.ParseFloat(t1d2.Text, 64)
		cap, _ := strconv.ParseFloat(t1cable.Text, 64)
		rt1, _ := strconv.ParseFloat(t1rt1.Text, 64)
		rt2, _ := strconv.ParseFloat(t1rt2.Text, 64)

		tv := make([]Trader, len(tnTraders))
		for i, t := range tnTraders {
			tv[i] = *t
		}
		bv := make([]Bid, len(tnBids))
		for i, b := range tnBids {
			bv[i] = *b
		}
		vv := make([]VirtualBid, len(tnVirts))
		for i, v := range tnVirts {
			vv[i] = *v
		}
		fv := make([]FTRBid, len(tnFTRs))
		for i, f := range tnFTRs {
			fv[i] = *f
		}

		sol := solveTwoNode(tv, bv, vv, d1, d2, cap, rt1, rt2, fv)

		for i := range tnTraders {
			tnTraders[i].GenProfit = sol.Traders[i].GenProfit
			tnTraders[i].VirtProfit = sol.Traders[i].VirtProfit
			tnTraders[i].FTRProfit = sol.Traders[i].FTRProfit
			tnTraders[i].Total = sol.Traders[i].Total
		}
		for i := range tnBids {
			for _, sb := range sol.Bids {
				if !sb.Virtual && tnBids[i].TraderIdx == sb.TraderIdx && tnBids[i].Node == sb.Node && tnBids[i].Price == sb.Price {
					tnBids[i].Dispatch = sb.Dispatch
					tnBids[i].Profit = sb.Profit
					break
				}
			}
		}
		for i := range tnVirts {
			tnVirts[i].Cleared = sol.Virtuals[i].Cleared
			tnVirts[i].NetPnL = sol.Virtuals[i].NetPnL
		}
		for i := range tnFTRs {
			tnFTRs[i].Entitle = sol.FTRs[i].Entitle
			tnFTRs[i].Payment = sol.FTRs[i].Payment
			tnFTRs[i].Profit = sol.FTRs[i].Profit
		}

		cong := "No"
		if sol.Congested {
			cong = "Yes"
		}
		t1result.SetText(fmt.Sprintf("DA: N1=$%.2f N2=$%.2f | RT: N1=$%.2f N2=$%.2f\nFlow: %.1f MW | Cong: %s | Rent: $%.0f | Pro-rata: %.0f%%",
			sol.Node1LMP, sol.Node2LMP, rt1, rt2, sol.Flow, cong, sol.RentPool, sol.ProRata*100))

		// Store for pedagogy
		lastFTRResult.Solved = true
		lastFTRResult.Node1LMP = sol.Node1LMP
		lastFTRResult.Node2LMP = sol.Node2LMP
		lastFTRResult.Flow = sol.Flow
		lastFTRResult.RentPool = sol.RentPool
		lastFTRResult.ProRata = sol.ProRata
		lastFTRResult.Congested = sol.Congested
		lastFTRResult.Traders = sol.Traders
		lastFTRResult.FTRs = sol.FTRs
		lastFTRResult.Virtuals = sol.Virtuals
		lastFTRResult.RTLMP1 = rt1
		lastFTRResult.RTLMP2 = rt2

		t1Refresh()
	})

	t1Refresh = func() {
		t1n1Box.Objects = []fyne.CanvasObject{widget.NewLabel("Node 1"), t1MakeBidForm(1)}
		t1n2Box.Objects = []fyne.CanvasObject{widget.NewLabel("Node 2"), t1MakeBidForm(2)}
		t1vBox.Objects = []fyne.CanvasObject{widget.NewLabel("Virtual"), t1MakeVirtForm()}
		t1fBox.Objects = []fyne.CanvasObject{widget.NewLabel("FTR (2→1)"), t1MakeFTRForm()}
		t1sBox.Objects = []fyne.CanvasObject{widget.NewLabel("Summary"), t1MakeSummary()}
		t1n1Box.Refresh()
		t1n2Box.Refresh()
		t1vBox.Refresh()
		t1fBox.Refresh()
		t1sBox.Refresh()
	}

	t1n1Box = container.NewVBox(widget.NewLabel("Node 1"), t1MakeBidForm(1))
	t1n2Box = container.NewVBox(widget.NewLabel("Node 2"), t1MakeBidForm(2))
	t1vBox = container.NewVBox(widget.NewLabel("Virtual"), t1MakeVirtForm())
	t1fBox = container.NewVBox(widget.NewLabel("FTR (2→1)"), t1MakeFTRForm())

	t1sBox = container.NewVBox(widget.NewLabel("Summary"), t1MakeSummary())

	t1Inputs := container.NewVBox(
		container.NewHBox(widget.NewLabel("N1 Demand:"), t1d1, widget.NewLabel("N2 Demand:"), t1d2, widget.NewLabel("Cable:"), t1cable),
		container.NewHBox(widget.NewLabel("RT N1:"), t1rt1, widget.NewLabel("RT N2:"), t1rt2, t1Solve),
		t1result)

	t1Left := container.NewVScroll(t1sBox)
	// Two-Node without FTR (3 columns: N1, N2, Virtual)
	t1MainNoFTR := container.NewGridWithColumns(3, container.NewVScroll(t1n1Box), container.NewVScroll(t1n2Box), container.NewVScroll(t1vBox))
	tab1aContent := container.NewBorder(t1Inputs, nil, t1Left, nil, t1MainNoFTR)
	// Two-Node with FTR (4 columns: N1, N2, Virtual, FTR)
	t1MainFTR := container.NewGridWithColumns(4, container.NewVScroll(t1n1Box), container.NewVScroll(t1n2Box), container.NewVScroll(t1vBox), container.NewVScroll(t1fBox))
	tab1bContent := container.NewBorder(t1Inputs, nil, t1Left, nil, t1MainFTR)

	// ===== TAB 2: Single-Node DA/RT =====
	tab2Traders := buildTraders()
	// Physical bids: all 12 players (MW, DA$, RT$) - from whiteboard
	tab2Phys := []*PhysBid{
		{0, 16, -3, -3, 0, 0, 0},   // SD: 16 @ -3
		{1, 18, 32, 32, 0, 0, 0},   // RK: 18 @ 32
		{2, 15, 20, 20, 0, 0, 0},   // KM: 15 @ 20
		{3, 0, 0, 0, 0, 0, 0},      // RM: no physical
		{4, 0, 0, 0, 0, 0, 0},      // RS: no physical
		{5, 0, 0, 0, 0, 0, 0},      // NK: no physical
		{6, 25, 26, 26, 0, 0, 0},   // MK: 25 @ 26
		{7, 0, 0, 0, 0, 0, 0},      // AF: no physical
		{8, 2, 32, 32, 0, 0, 0},    // OT: 2 @ 32
		{9, 5, 30, 30, 0, 0, 0},    // SF: 5 @ 30
		{10, 25, 34, 34, 0, 0, 0},  // VK: 25 @ 34
		{11, 30, 35, 35, 0, 0, 0},  // MPC: 30 @ 35
	}
	// Virtual bids: all 12 players (Type, MW, $)
	tab2Virts := []*VirtBid{
		{0, "", 0, 0, 0, 0},       // SD
		{1, "DEC", 8, 19, 0, 0},   // RK
		{2, "", 0, 0, 0, 0},       // KM
		{3, "INC", 85, 20, 0, 0},  // RM
		{4, "DEC", 15, 25, 0, 0},  // RS
		{5, "INC", 25, 25, 0, 0},  // NK
		{6, "", 0, 0, 0, 0},       // MK
		{7, "INC", 37, 30, 0, 0},  // AF
		{8, "DEC", 15, 30, 0, 0},  // OT
		{9, "", 0, 0, 0, 0},       // SF
		{10, "", 0, 0, 0, 0},      // VK
		{11, "INC", 25, 40, 0, 0}, // MPC
	}

	t2daDemand := newNarrowEntry()
	t2daDemand.SetText("80")
	t2rtDemand := newNarrowEntry()
	t2rtDemand.SetText("80")
	t2incCap := newNarrowEntry()
	t2incCap.SetText("25")
	t2decCap := newNarrowEntry()
	t2decCap.SetText("25")
	t2result := widget.NewLabel("")
	t2result.Wrapping = fyne.TextWrapWord

	var t2Refresh func()
	var t2pBox, t2vBox, t2sBox *fyne.Container

	t2MakePhysForm := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(7,
			widget.NewLabel("Player"), widget.NewLabel("MW"), widget.NewLabel("DA$"), widget.NewLabel("RT$"),
			widget.NewLabel("DA Dsp"), widget.NewLabel("RT Dsp"), widget.NewLabel("Profit"))}
		// Sort by DA price (merit order)
		sorted := make([]*PhysBid, len(tab2Phys))
		copy(sorted, tab2Phys)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].DAPrice < sorted[j].DAPrice })
		for _, p := range sorted {
			pb := p
			lbl := widget.NewLabel(players[pb.TraderIdx])
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.0f", pb.MW))
			mw.OnChanged = func(s string) { pb.MW, _ = strconv.ParseFloat(s, 64) }
			dap := newNarrowEntry()
			dap.SetText(fmt.Sprintf("%.0f", pb.DAPrice))
			dap.OnChanged = func(s string) { pb.DAPrice, _ = strconv.ParseFloat(s, 64) }
			rtp := newNarrowEntry()
			rtp.SetText(fmt.Sprintf("%.0f", pb.RTPrice))
			rtp.OnChanged = func(s string) { pb.RTPrice, _ = strconv.ParseFloat(s, 64) }
			rows = append(rows, container.NewGridWithColumns(7, lbl, mw, dap, rtp,
				widget.NewLabel(fmt.Sprintf("%.1f", pb.DADisp)),
				widget.NewLabel(fmt.Sprintf("%.1f", pb.RTDisp)),
				widget.NewLabel(fmt.Sprintf("$%.0f", pb.Profit))))
		}
		return container.NewVBox(rows...)
	}

	t2MakeVirtForm := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(6,
			widget.NewLabel("Player"), widget.NewLabel("Type"), widget.NewLabel("MW"),
			widget.NewLabel("$"), widget.NewLabel("Clr"), widget.NewLabel("PnL"))}
		// Sort by price (merit order)
		sorted := make([]*VirtBid, len(tab2Virts))
		copy(sorted, tab2Virts)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].Price < sorted[j].Price })
		for _, v := range sorted {
			vb := v
			lbl := widget.NewLabel(players[vb.TraderIdx])
			ty := widget.NewSelect([]string{"", "INC", "DEC"}, func(s string) { vb.Type = s })
			ty.SetSelected(vb.Type)
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.0f", vb.MW))
			mw.OnChanged = func(s string) { vb.MW, _ = strconv.ParseFloat(s, 64) }
			pr := newNarrowEntry()
			pr.SetText(fmt.Sprintf("%.0f", vb.Price))
			pr.OnChanged = func(s string) { vb.Price, _ = strconv.ParseFloat(s, 64) }
			rows = append(rows, container.NewGridWithColumns(6, lbl, ty, mw, pr,
				widget.NewLabel(fmt.Sprintf("%.0f", vb.Cleared)),
				widget.NewLabel(fmt.Sprintf("$%.0f", vb.NetPnL))))
		}
		return container.NewVBox(rows...)
	}

	t2MakeSummary := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(4,
			widget.NewLabel("Trader"), widget.NewLabel("Gen"), widget.NewLabel("Virt"), widget.NewLabel("Total"))}
		for _, t := range tab2Traders {
			rows = append(rows, container.NewGridWithColumns(4,
				widget.NewLabel(t.Name),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.GenProfit)),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.VirtProfit)),
				widget.NewLabel(fmt.Sprintf("$%.0f", t.Total))))
		}
		return container.NewVBox(rows...)
	}

	t2Solve := widget.NewButton("Solve", func() {
		daD, _ := strconv.ParseFloat(t2daDemand.Text, 64)
		rtD, _ := strconv.ParseFloat(t2rtDemand.Text, 64)
		incC, _ := strconv.ParseFloat(t2incCap.Text, 64)
		decC, _ := strconv.ParseFloat(t2decCap.Text, 64)

		tv := make([]Trader, len(tab2Traders))
		for i, t := range tab2Traders {
			tv[i] = *t
		}
		pv := make([]PhysBid, len(tab2Phys))
		for i, p := range tab2Phys {
			pv[i] = *p
		}
		vv := make([]VirtBid, len(tab2Virts))
		for i, v := range tab2Virts {
			vv[i] = *v
		}

		daLMP, rtLMP, outP, outV, outT := solveDART2(tv, pv, vv, daD, rtD, incC, decC)

		for i := range tab2Traders {
			tab2Traders[i].GenProfit = outT[i].GenProfit
			tab2Traders[i].VirtProfit = outT[i].VirtProfit
			tab2Traders[i].Total = outT[i].Total
		}
		for i := range tab2Phys {
			tab2Phys[i].DADisp = outP[i].DADisp
			tab2Phys[i].RTDisp = outP[i].RTDisp
			tab2Phys[i].Profit = outP[i].Profit
		}
		for i := range tab2Virts {
			tab2Virts[i].Cleared = outV[i].Cleared
			tab2Virts[i].NetPnL = outV[i].NetPnL
		}

		t2result.SetText(fmt.Sprintf("DA LMP: $%.2f | RT LMP: $%.2f | Spread: $%.2f", daLMP, rtLMP, daLMP-rtLMP))

		// Store for pedagogy
		lastDARTResult.Solved = true
		lastDARTResult.DALMP = daLMP
		lastDARTResult.RTLMP = rtLMP
		lastDARTResult.Traders = outT
		lastDARTResult.Virtuals = outV
		lastDARTResult.DADemand = daD
		lastDARTResult.RTDemand = rtD

		t2Refresh()
	})

	t2Refresh = func() {
		t2pBox.Objects = []fyne.CanvasObject{widget.NewLabel("Physical Bids"), t2MakePhysForm()}
		t2vBox.Objects = []fyne.CanvasObject{widget.NewLabel("Virtual Bids"), t2MakeVirtForm()}
		t2sBox.Objects = []fyne.CanvasObject{widget.NewLabel("Summary"), t2MakeSummary()}
		t2pBox.Refresh()
		t2vBox.Refresh()
		t2sBox.Refresh()
	}

	t2pBox = container.NewVBox(widget.NewLabel("Physical Bids"), t2MakePhysForm())
	t2vBox = container.NewVBox(widget.NewLabel("Virtual Bids"), t2MakeVirtForm())
	t2sBox = container.NewVBox(widget.NewLabel("Summary"), t2MakeSummary())

	t2Inputs := container.NewVBox(
		container.NewHBox(widget.NewLabel("DA Demand:"), t2daDemand, widget.NewLabel("RT Demand:"), t2rtDemand,
			widget.NewLabel("INC Cap:"), t2incCap, widget.NewLabel("DEC Cap:"), t2decCap, t2Solve),
		t2result)

	t2Left := container.NewVScroll(t2sBox)
	t2Main := container.NewGridWithColumns(2, container.NewVScroll(t2pBox), container.NewVScroll(t2vBox))
	tab2Content := container.NewBorder(t2Inputs, nil, t2Left, nil, t2Main)

	// ===== TEACHER TAB =====
	var teacherRefresh func()
	var playerBox, genBox *fyne.Container

	playerEntries := make([]*narrowEntry, len(players))
	for i := range players {
		idx := i
		e := newNarrowEntry()
		e.SetText(players[idx])
		e.OnChanged = func(s string) { players[idx] = s }
		playerEntries[i] = e
	}

	makePlayerList := func() *fyne.Container {
		rows := []fyne.CanvasObject{widget.NewLabel("Players")}
		for i, e := range playerEntries {
			rows = append(rows, container.NewHBox(widget.NewLabel(fmt.Sprintf("%d.", i+1)), e))
		}
		return container.NewVBox(rows...)
	}

	makeGenList := func() *fyne.Container {
		rows := []fyne.CanvasObject{container.NewGridWithColumns(6,
			widget.NewLabel("Gen"), widget.NewLabel("MW"), widget.NewLabel("$/MWh"), widget.NewLabel("Node"), widget.NewLabel("Owner"), widget.NewLabel(""))}
		for _, g := range generators {
			gen := g
			nm := newNarrowEntry()
			nm.SetText(gen.Name)
			nm.OnChanged = func(s string) { gen.Name = s }
			mw := newNarrowEntry()
			mw.SetText(fmt.Sprintf("%.0f", gen.MW))
			mw.OnChanged = func(s string) { gen.MW, _ = strconv.ParseFloat(s, 64) }
			cost := newNarrowEntry()
			cost.SetText(fmt.Sprintf("%.0f", gen.Cost))
			cost.OnChanged = func(s string) { gen.Cost, _ = strconv.ParseFloat(s, 64) }
			node := widget.NewSelect([]string{"1", "2"}, func(s string) { gen.Node, _ = strconv.Atoi(s) })
			node.SetSelected(fmt.Sprintf("%d", gen.Node))
			opts := append([]string{"(none)"}, players...)
			owner := widget.NewSelect(opts, func(s string) {
				if s == "(none)" {
					gen.OwnerIdx = -1
				} else {
					for i, p := range players {
						if p == s {
							gen.OwnerIdx = i
							break
						}
					}
				}
			})
			if gen.OwnerIdx >= 0 && gen.OwnerIdx < len(players) {
				owner.SetSelected(players[gen.OwnerIdx])
			} else {
				owner.SetSelected("(none)")
			}
			rows = append(rows, container.NewGridWithColumns(6, nm, mw, cost, node, owner, widget.NewLabel("")))
		}
		return container.NewVBox(rows...)
	}

	addGenBtn := widget.NewButton("Add Generator", func() {
		generators = append(generators, &Generator{fmt.Sprintf("Gen-%d", len(generators)+1), 10, 25, -1, 1})
		teacherRefresh()
	})

	teacherRefresh = func() {
		playerBox.Objects = []fyne.CanvasObject{makePlayerList()}
		genBox.Objects = []fyne.CanvasObject{makeGenList(), addGenBtn}
		playerBox.Refresh()
		genBox.Refresh()
	}

	playerBox = container.NewVBox(makePlayerList())
	genBox = container.NewVBox(makeGenList(), addGenBtn)

	teacherContent := container.NewHSplit(
		container.NewVScroll(playerBox),
		container.NewVScroll(genBox),
	)
	teacherContent.SetOffset(0.25)

	// ===== TAB 4: Pedagogy (renamed from TAB 3) =====
	ftrExplain := widget.NewLabel("")
	ftrExplain.Wrapping = fyne.TextWrapWord
	dartExplain := widget.NewLabel("")
	dartExplain.Wrapping = fyne.TextWrapWord

	refreshPedagogy := func() {
		// FTR explanation
		if lastFTRResult.Solved {
			r := lastFTRResult
			txt := "FINANCIAL TRANSMISSION RIGHTS (FTRs)\n"
			txt += "====================================\n\n"
			txt += "THE PROBLEM: Transmission Congestion Risk\n\n"
			if r.Congested {
				txt += fmt.Sprintf("In this scenario, the transmission line between nodes is CONGESTED.\n")
				txt += fmt.Sprintf("Node 1 clears at $%.2f/MWh, Node 2 at $%.2f/MWh.\n", r.Node1LMP, r.Node2LMP)
				txt += fmt.Sprintf("The price difference of $%.2f/MWh creates risk for market participants.\n\n", r.Node1LMP-r.Node2LMP)
			} else {
				txt += "The line is NOT congested, so both nodes clear at the same price.\n"
				txt += "FTRs have zero value when there's no congestion.\n\n"
			}

			txt += "HOW FTRs CREATE HEDGING VALUE\n\n"
			txt += fmt.Sprintf("Power flows %.1f MW from Node 1 to Node 2.\n", r.Flow)
			txt += fmt.Sprintf("This creates a 'congestion rent' pool of $%.0f (= %.1f MW × $%.2f price diff).\n\n", r.RentPool, math.Abs(r.Flow), math.Abs(r.Node1LMP-r.Node2LMP))

			txt += "FTR holders in the 2→1 direction receive payments from this rent pool:\n\n"
			for _, f := range r.FTRs {
				name := "Unknown"
				if f.TraderIdx >= 0 && f.TraderIdx < len(r.Traders) {
					name = r.Traders[f.TraderIdx].Name
				}
				if f.Entitle > 0 {
					txt += fmt.Sprintf("• %s: Bought %0.f MW FTR at $%.0f/MW.\n", name, f.MW, f.BidPrice)
					txt += fmt.Sprintf("  Entitlement: $%.0f (%.0f MW × $%.2f LMP diff)\n", f.Entitle, f.MW, r.Node1LMP-r.Node2LMP)
					txt += fmt.Sprintf("  Payment: $%.0f (%.0f%% pro-rata), Cost: $%.0f, Profit: $%.0f\n\n", f.Payment, r.ProRata*100, f.Cost, f.Profit)
				}
			}

			if r.ProRata < 1.0 {
				txt += fmt.Sprintf("NOTE: Total FTR entitlements exceed the rent pool.\n")
				txt += fmt.Sprintf("Payments are scaled to %.0f%% (pro-rata allocation).\n\n", r.ProRata*100)
			}

			txt += "EFFICIENCY BENEFIT\n\n"
			txt += "Without FTRs: Generators/loads cannot hedge congestion risk.\n"
			txt += "A generator at Node 2 selling to a customer at Node 1 faces uncertain costs.\n\n"
			txt += "With FTRs: The generator can buy FTRs 2→1 to lock in transmission costs.\n"
			txt += "If congestion occurs, FTR payments offset the higher cost of delivering power.\n"
			txt += "This enables long-term contracts and reduces market risk premiums."

			ftrExplain.SetText(txt)
		} else {
			ftrExplain.SetText("Run the Two-Node FTR simulation first to see the explanation with actual data.")
		}

		// DA/RT Virtual explanation
		if lastDARTResult.Solved {
			r := lastDARTResult
			txt := "VIRTUAL BIDDING (INC/DEC)\n"
			txt += "=========================\n\n"
			txt += "THE PROBLEM: Day-Ahead vs Real-Time Price Divergence\n\n"
			txt += fmt.Sprintf("Day-Ahead LMP: $%.2f/MWh\n", r.DALMP)
			txt += fmt.Sprintf("Real-Time LMP: $%.2f/MWh\n", r.RTLMP)
			txt += fmt.Sprintf("Spread: $%.2f/MWh\n\n", r.DALMP-r.RTLMP)

			if r.DALMP > r.RTLMP {
				txt += "DA price is HIGHER than RT. INC bids profit, DEC bids lose.\n\n"
			} else if r.DALMP < r.RTLMP {
				txt += "DA price is LOWER than RT. DEC bids profit, INC bids lose.\n\n"
			} else {
				txt += "DA and RT prices are equal. Virtual bids break even.\n\n"
			}

			txt += "HOW VIRTUAL BIDS WORK\n\n"
			txt += "INC (Increment/Virtual Supply):\n"
			txt += "• Sells power in Day-Ahead market (receives DA LMP)\n"
			txt += "• Must buy back in Real-Time (pays RT LMP)\n"
			txt += "• Profits when DA > RT (bet that DA is too high)\n\n"
			txt += "DEC (Decrement/Virtual Demand):\n"
			txt += "• Buys power in Day-Ahead market (pays DA LMP)\n"
			txt += "• Sells in Real-Time (receives RT LMP)\n"
			txt += "• Profits when RT > DA (bet that DA is too low)\n\n"

			txt += "RESULTS FROM THIS SIMULATION:\n\n"
			for _, v := range r.Virtuals {
				if v.Type == "" || v.MW == 0 {
					continue
				}
				name := "Unknown"
				if v.TraderIdx >= 0 && v.TraderIdx < len(r.Traders) {
					name = r.Traders[v.TraderIdx].Name
				}
				if v.Cleared > 0 {
					txt += fmt.Sprintf("• %s placed %s bid: %.0f MW at $%.0f\n", name, v.Type, v.MW, v.Price)
					if v.Type == "INC" {
						txt += fmt.Sprintf("  Sold DA at $%.2f, bought RT at $%.2f\n", r.DALMP, r.RTLMP)
					} else {
						txt += fmt.Sprintf("  Bought DA at $%.2f, sold RT at $%.2f\n", r.DALMP, r.RTLMP)
					}
					txt += fmt.Sprintf("  Net P&L: $%.0f\n\n", v.NetPnL)
				} else {
					txt += fmt.Sprintf("• %s %s bid (%.0f MW at $%.0f) did NOT clear\n\n", name, v.Type, v.MW, v.Price)
				}
			}

			txt += "EFFICIENCY BENEFIT\n\n"
			txt += "Virtual bidders are financial speculators with no physical assets.\n"
			txt += "They profit by correctly predicting DA-RT price differences.\n\n"
			txt += "This improves market efficiency by:\n"
			txt += "1. PRICE CONVERGENCE: Virtual trades push DA prices toward expected RT\n"
			txt += "2. RISK TRANSFER: Physical participants can offset positions to virtuals\n"
			txt += "3. LIQUIDITY: More market participants = tighter bid-ask spreads\n"
			txt += "4. INFORMATION: Prices better reflect all available information"

			dartExplain.SetText(txt)
		} else {
			dartExplain.SetText("Run the DA/RT Virtual simulation first to see the explanation with actual data.")
		}
	}

	refreshBtn := widget.NewButton("Refresh Explanations", func() { refreshPedagogy() })

	ftrSection := container.NewVBox(widget.NewLabelWithStyle("FTR Mechanism", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), ftrExplain)
	dartSection := container.NewVBox(widget.NewLabelWithStyle("Virtual Bidding Mechanism", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), dartExplain)

	tab3Content := container.NewBorder(
		container.NewVBox(widget.NewLabel("Run simulations in the other tabs, then refresh to see explanations with your data."), refreshBtn),
		nil, nil, nil,
		container.NewVScroll(container.NewVBox(ftrSection, widget.NewSeparator(), dartSection)),
	)

	// ===== TABS =====
	tabs := container.NewAppTabs(
		container.NewTabItem("Teacher", teacherContent),
		container.NewTabItem("Two-Node", tab1aContent),
		container.NewTabItem("Two-Node + FTR", tab1bContent),
		container.NewTabItem("DA/RT Virtual", tab2Content),
		container.NewTabItem("Pedagogy", tab3Content),
	)

	w.SetContent(tabs)

	// Auto-solve on startup
	t1Solve.OnTapped()
	t2Solve.OnTapped()
	refreshPedagogy()

	w.ShowAndRun()
}

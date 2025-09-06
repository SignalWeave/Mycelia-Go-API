package mycelia

// -------Channel Selection Strategies------------------------------------------

type SEL_STRAT uint8

const (
	SEL_STRAT_RANDOM     SEL_STRAT = 0
	SEL_STRAT_ROUNDROBIN SEL_STRAT = 1
	SEL_STRAT_PUBSUB     SEL_STRAT = 2
)

var selStratName = map[SEL_STRAT]string{
	SEL_STRAT_RANDOM:     "random",
	SEL_STRAT_ROUNDROBIN: "round-robin",
	SEL_STRAT_PUBSUB:     "pub-sub",
}

func (ss SEL_STRAT) String() string {
	return selStratName[ss]
}

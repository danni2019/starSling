package strategy

type Strategy interface {
	Name() string
}

type Factory func() Strategy

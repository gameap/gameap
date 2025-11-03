package services

import "context"

type NilTransactionManager struct{}

func NewNilTransactionManager() *NilTransactionManager {
	return &NilTransactionManager{}
}

func (ntm *NilTransactionManager) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

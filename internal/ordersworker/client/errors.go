package client

import (
	"errors"
	"fmt"
)

var (
	ErrUnavailable      = errors.New("orders-api no disponible")
	ErrTimeout          = errors.New("timeout de orders-api")
	ErrNotFound         = errors.New("pedido no encontrado")
	ErrInvalidStatus    = errors.New("estado inválido")
	ErrTemporary        = errors.New("error temporal de orders-api")
	ErrPermanent        = errors.New("error permanente de orders-api")
	ErrResponseTooLarge = errors.New("respuesta demasiado grande")
)

type HTTPError struct {
	Status int
	Kind   error
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("orders-api respondió HTTP %d: %v", e.Status, e.Kind)
}
func (e *HTTPError) Unwrap() error { return e.Kind }

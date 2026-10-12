//go:build !e2e

package server

import "github.com/go-chi/chi/v5"

func registerE2ETestRoutes(chi.Router) {}

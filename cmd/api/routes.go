package main

import (
	"github.com/julienschmidt/httprouter"
	"net/http"
)

func (app *application) routes() http.Handler {
	router := httprouter.New()

	router.NotFound = http.HandlerFunc(app.notFoundResponse)
	router.MethodNotAllowed = http.HandlerFunc(app.methodNotAllowedResponse)

	router.HandlerFunc(http.MethodGet, "/v1/healthcheck", app.healthcheckHandler)

	router.HandlerFunc(http.MethodGet, "/v1/animes", app.requirePermission("animes:read", app.listAnimesHandler))
	router.HandlerFunc(http.MethodPost, "/v1/animes", app.requirePermission("animes:write", app.createAnimeHandler))
	router.HandlerFunc(http.MethodGet, "/v1/animes/:id", app.requirePermission("animes:read", app.showAnimeHandler))
	router.HandlerFunc(http.MethodPatch, "/v1/animes/:id", app.requirePermission("animes:write", app.updateAnimeHandler))
	router.HandlerFunc(http.MethodDelete, "/v1/animes/:id", app.requirePermission("animes:write", app.deleteAnimeHandler))

	router.HandlerFunc(http.MethodPost, "/v1/users", app.registerUserHandler)
	router.HandlerFunc(http.MethodPut, "/v1/users/activated", app.activateUserHandler)

	router.HandlerFunc(http.MethodPost, "/v1/tokens/authentication", app.createAuthenticationTokenHandler)

	return app.recoverPanic(app.rateLimit(app.authenticate(router)))

}

package main

import (
	"context"
	"log"

	"github.com/gin-gonic/gin"

	"tf200/todoapp/config"
	"tf200/todoapp/internal/auth"
	"tf200/todoapp/internal/database"
	"tf200/todoapp/internal/list"
	"tf200/todoapp/internal/middleware"
	"tf200/todoapp/internal/note"
	"tf200/todoapp/internal/task"
)

func main() {
	cfg := config.Load()

	client, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("failed to connect to mongo: %v", err)
	}
	db := client.Database("todoapp")

	authRepo := auth.NewRepository(db)
	if err := authRepo.EnsureIndexes(context.Background()); err != nil {
		log.Fatalf("failed to create user indexes: %v", err)
	}
	authSvc := auth.NewService(authRepo, cfg.JWTSecret)
	authH := auth.NewHandler(authSvc)

	listRepo := list.NewRepository(db)
	listSvc := list.NewService(listRepo)
	listH := list.NewHandler(listSvc)

	taskRepo := task.NewRepository(db)
	taskSvc := task.NewService(taskRepo, listSvc)
	taskH := task.NewHandler(taskSvc)

	noteRepo := note.NewRepository(db)
	if err := noteRepo.EnsureIndexes(context.Background()); err != nil {
		log.Fatalf("failed to create note indexes: %v", err)
	}
	noteH := note.NewHandler(note.NewService(noteRepo))

	r := gin.Default()

	r.POST("/login", authH.Login)
	r.POST("/register", authH.Register)
	r.POST("/refresh", authH.Refresh)
	r.POST("/logout", authH.Logout)

	api := r.Group("/api")
	api.Use(middleware.AuthRequired(cfg.JWTSecret, authRepo))
	{
		api.GET("/me", authH.Me)

		tokens := api.Group("/tokens", middleware.RequireJWT())
		{
			tokens.POST("", authH.CreateAPIToken)
			tokens.GET("", authH.ListAPITokens)
			tokens.DELETE("/:id", authH.RevokeAPIToken)
		}

		api.GET("/lists", listH.GetAll)
		api.POST("/lists", listH.Create)
		api.GET("/lists/:id", listH.GetByID)
		api.PUT("/lists/:id", listH.Update)
		api.DELETE("/lists/:id", listH.Delete)

		api.POST("/lists/:id/tasks", taskH.Create)
		api.GET("/lists/:id/tasks", taskH.GetByList)
		api.GET("/tasks/:id", taskH.GetByID)
		api.PUT("/tasks/:id", taskH.Update)
		api.DELETE("/tasks/:id", taskH.Delete)

		api.GET("/notes", noteH.GetAll)
		api.POST("/notes", noteH.Create)
		api.GET("/notes/:id", noteH.GetByID)
		api.PUT("/notes/:id", noteH.Update)
		api.DELETE("/notes/:id", noteH.Delete)
	}

	r.Run(":" + cfg.Port)
}

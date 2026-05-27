package main

import (
	"log"

	"github.com/gin-gonic/gin"

	"tf200/todoapp/config"
	"tf200/todoapp/internal/auth"
	"tf200/todoapp/internal/database"
	"tf200/todoapp/internal/list"
	"tf200/todoapp/internal/middleware"
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
	authSvc := auth.NewService(authRepo, cfg.JWTSecret)
	authH := auth.NewHandler(authSvc)

	listRepo := list.NewRepository(db)
	listSvc := list.NewService(listRepo)
	listH := list.NewHandler(listSvc)

	taskRepo := task.NewRepository(db)
	taskSvc := task.NewService(taskRepo, listSvc)
	taskH := task.NewHandler(taskSvc)

	r := gin.Default()

	r.POST("/login", authH.Login)

	auth := r.Group("/api")
	auth.Use(middleware.AuthRequired(cfg.JWTSecret))
	{
		auth.GET("/lists", listH.GetAll)
		auth.POST("/lists", listH.Create)
		auth.GET("/lists/:id", listH.GetByID)
		auth.PUT("/lists/:id", listH.Update)
		auth.DELETE("/lists/:id", listH.Delete)

		auth.POST("/lists/:listId/tasks", taskH.Create)
		auth.GET("/lists/:listId/tasks", taskH.GetByList)
		auth.GET("/tasks/:id", taskH.GetByID)
		auth.PUT("/tasks/:id", taskH.Update)
		auth.DELETE("/tasks/:id", taskH.Delete)
	}

	r.Run(":" + cfg.Port)
}

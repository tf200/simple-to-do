package main

import (
	"context"
	"log"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"tf200/todoapp/config"
	"tf200/todoapp/internal/attachment"
	"tf200/todoapp/internal/auth"
	"tf200/todoapp/internal/database"
	"tf200/todoapp/internal/list"
	"tf200/todoapp/internal/middleware"
	"tf200/todoapp/internal/note"
	"tf200/todoapp/internal/stage"
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

	stageSvc := stage.NewService(stage.NewRepository(db))
	stageH := stage.NewHandler(stageSvc)

	taskRepo := task.NewRepository(db)
	taskSvc := task.NewService(taskRepo, listSvc, stageSvc)
	taskH := task.NewHandler(taskSvc)

	noteRepo := note.NewRepository(db)
	if err := noteRepo.EnsureIndexes(context.Background()); err != nil {
		log.Fatalf("failed to create note indexes: %v", err)
	}
	noteSvc := note.NewService(noteRepo)
	noteH := note.NewHandler(noteSvc)

	// Deleting a list deletes its tasks (they'd be unreachable otherwise).
	listSvc.BeforeDelete(taskSvc.DeleteByList)

	attachH := setUpAttachments(cfg, db, taskSvc, noteSvc)

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

		api.GET("/stages", stageH.Get)
		api.PUT("/stages", stageH.Put)

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

		api.GET("/tasks/:id/attachments", attachH.ListFor(attachment.KindTask))
		api.POST("/tasks/:id/attachments", attachH.StartFor(attachment.KindTask))
		api.GET("/notes/:id/attachments", attachH.ListFor(attachment.KindNote))
		api.POST("/notes/:id/attachments", attachH.StartFor(attachment.KindNote))
		api.POST("/attachments/:id/complete", attachH.Complete)
		api.GET("/attachments/:id/download", attachH.Download)
		api.DELETE("/attachments/:id", attachH.Delete)
	}

	r.Run(":" + cfg.Port)
}

// setUpAttachments connects the attachment store and makes deleting a task or note delete its
// files. With no store configured, the endpoints answer 503 and deletes go on as before.
func setUpAttachments(cfg *config.Config, db *mongo.Database, tasks *task.Service, notes *note.Service) *attachment.Handler {
	repo := attachment.NewRepository(db)
	if err := repo.EnsureIndexes(context.Background()); err != nil {
		log.Fatalf("failed to create attachment indexes: %v", err)
	}
	parents := parentLookup{tasks: tasks, notes: notes}
	if !cfg.S3.Configured() {
		log.Printf("attachments: off (no S3_ENDPOINT / S3_ACCESS_KEY / S3_SECRET_KEY)")
		return attachment.NewHandler(attachment.NewService(repo, nil, parents, false))
	}
	store, err := attachment.NewS3Store(attachment.S3Config{
		Endpoint:  cfg.S3.Endpoint,
		PublicURL: cfg.S3.PublicURL,
		Region:    cfg.S3.Region,
		Bucket:    cfg.S3.Bucket,
		AccessKey: cfg.S3.AccessKey,
		SecretKey: cfg.S3.SecretKey,
	})
	if err != nil {
		log.Fatalf("attachments: %v", err)
	}
	if !store.CanSign() {
		log.Printf("attachments: off until S3_PUBLIC_URL is set (the store is used for cleanup only)")
	}
	svc := attachment.NewService(repo, store, parents, store.CanSign())
	tasks.BeforeDelete(func(ctx context.Context, ids []primitive.ObjectID) error {
		return svc.DeleteParents(ctx, attachment.KindTask, ids)
	})
	notes.BeforeDelete(func(ctx context.Context, ids []primitive.ObjectID) error {
		return svc.DeleteParents(ctx, attachment.KindNote, ids)
	})
	go svc.SweepEvery(context.Background(), 15*time.Minute)
	return attachment.NewHandler(svc)
}

// parentLookup tells the attachments whether a task or note belongs to a user.
type parentLookup struct {
	tasks *task.Service
	notes *note.Service
}

func (p parentLookup) Owned(ctx context.Context, userID primitive.ObjectID, kind string, id primitive.ObjectID) error {
	var err error
	switch kind {
	case attachment.KindTask:
		_, err = p.tasks.GetByID(ctx, userID, id)
	case attachment.KindNote:
		_, err = p.notes.GetByID(ctx, userID, id)
	default:
		return attachment.ErrNotFound
	}
	if err != nil {
		// The task and note services don't tell "missing" from "someone else's": both are a 404.
		return attachment.ErrNotFound
	}
	return nil
}

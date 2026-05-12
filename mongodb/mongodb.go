// Package mongodb provides a storage adapter backed by a MongoDB database.
//
// It implements [storage.Adapter] and [storage.ConversationStore]. Collections
// and indexes are created implicitly on first use.
//
// Basic usage:
//
//	store, err := mongodb.Open(ctx, mongodb.Config{
//	    URI:      "mongodb://localhost:27017",
//	    Database: "mtgo",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer store.Close()
package mongodb

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/mtgo-labs/storage"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoDB is a storage adapter backed by a MongoDB database.
type MongoDB struct {
	client *mongo.Client
	db     *mongo.Database

	convOnce  sync.Once
	convReady bool
}

var (
	_ storage.Adapter           = (*MongoDB)(nil)
	_ storage.ConversationStore = (*MongoDB)(nil)
)

// Config holds MongoDB connection parameters.
type Config struct {
	URI      string
	Database string
}

// Open connects to a MongoDB cluster and verifies connectivity.
// Collections are created implicitly on first use.
func Open(ctx context.Context, cfg Config) (*MongoDB, error) {
	client, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		client.Disconnect(ctx)
		return nil, err
	}
	db := client.Database(cfg.Database)

	// Create indexes for peer lookups.
	_, _ = db.Collection("peers").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "username", Value: 1}},
	})
	_, _ = db.Collection("conversations").Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "chat_id", Value: 1}, {Key: "user_id", Value: 1}},
	})

	return &MongoDB{client: client, db: db}, nil
}

func (m *MongoDB) ensureConvColl(ctx context.Context) {
	m.convOnce.Do(func() {
		m.convReady = true
	})
}

// --- SessionStore ---

func (m *MongoDB) LoadSession() (*storage.Session, error) {
	coll := m.db.Collection("sessions")
	var doc struct {
		DC            int    `bson:"dc_id"`
		APIID         int    `bson:"api_id"`
		APIHash       string `bson:"api_hash"`
		TestMode      int    `bson:"test_mode"`
		AuthKey       []byte `bson:"auth_key"`
		State         []byte `bson:"state"`
		UserID        int64  `bson:"user_id"`
		IsBot         int    `bson:"is_bot"`
		FirstName     string `bson:"first_name"`
		LastName      string `bson:"last_name"`
		Username      string `bson:"username"`
		Date          int    `bson:"date"`
		ServerAddress string `bson:"server_address"`
		Port          int    `bson:"port"`
	}
	err := coll.FindOne(context.Background(), bson.M{}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Session{
		DC: doc.DC, APIID: doc.APIID, APIHash: doc.APIHash, TestMode: doc.TestMode != 0,
		AuthKey: doc.AuthKey, State: doc.State, UserID: doc.UserID, IsBot: doc.IsBot != 0,
		FirstName: doc.FirstName, LastName: doc.LastName, Username: doc.Username,
		Date: doc.Date, Addr: doc.ServerAddress, Port: doc.Port,
	}, nil
}

func (m *MongoDB) SaveSession(s *storage.Session) error {
	coll := m.db.Collection("sessions")
	tm := 0
	if s.TestMode {
		tm = 1
	}
	ib := 0
	if s.IsBot {
		ib = 1
	}
	doc := bson.M{
		"api_id": s.APIID, "api_hash": s.APIHash, "test_mode": tm,
		"auth_key": s.AuthKey, "state": s.State, "user_id": s.UserID, "is_bot": ib,
		"first_name": s.FirstName, "last_name": s.LastName, "username": s.Username,
		"date": s.Date, "server_address": s.Addr, "port": s.Port,
	}
	_, err := coll.ReplaceOne(context.Background(), bson.M{}, doc, options.Replace().SetUpsert(true))
	return err
}

// --- PeerStore ---

func (m *MongoDB) SavePeer(p *storage.Peer) error {
	coll := m.db.Collection("peers")
	ib := 0
	if p.IsBot {
		ib = 1
	}
	doc := bson.M{
		"type": p.Type, "access_hash": p.AccessHash, "username": p.Username,
		"usernames": p.Usernames, "first_name": p.FirstName, "last_name": p.LastName,
		"phone_number": p.PhoneNumber, "is_bot": ib, "photo_id": p.PhotoID,
		"language": p.Language, "last_updated": p.LastUpdated,
	}
	_, err := coll.ReplaceOne(context.Background(), bson.M{"_id": p.ID}, doc, options.Replace().SetUpsert(true))
	return err
}

func (m *MongoDB) GetPeer(id int64) (*storage.Peer, error) {
	coll := m.db.Collection("peers")
	var doc struct {
		ID          int64  `bson:"_id"`
		Type        int    `bson:"type"`
		AccessHash  int64  `bson:"access_hash"`
		Username    string `bson:"username"`
		Usernames   string `bson:"usernames"`
		FirstName   string `bson:"first_name"`
		LastName    string `bson:"last_name"`
		PhoneNumber string `bson:"phone_number"`
		IsBot       int    `bson:"is_bot"`
		PhotoID     int64  `bson:"photo_id"`
		Language    string `bson:"language"`
		LastUpdated int64  `bson:"last_updated"`
	}
	err := coll.FindOne(context.Background(), bson.M{"_id": id}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Peer{
		ID: doc.ID, Type: storage.PeerType(doc.Type), AccessHash: doc.AccessHash,
		Username: doc.Username, Usernames: doc.Usernames, FirstName: doc.FirstName,
		LastName: doc.LastName, PhoneNumber: doc.PhoneNumber, IsBot: doc.IsBot != 0,
		PhotoID: doc.PhotoID, Language: doc.Language, LastUpdated: doc.LastUpdated,
	}, nil
}

func (m *MongoDB) GetPeerByUsername(username string) (*storage.Peer, error) {
	coll := m.db.Collection("peers")
	var doc struct {
		ID          int64  `bson:"_id"`
		Type        int    `bson:"type"`
		AccessHash  int64  `bson:"access_hash"`
		Username    string `bson:"username"`
		Usernames   string `bson:"usernames"`
		FirstName   string `bson:"first_name"`
		LastName    string `bson:"last_name"`
		PhoneNumber string `bson:"phone_number"`
		IsBot       int    `bson:"is_bot"`
		PhotoID     int64  `bson:"photo_id"`
		Language    string `bson:"language"`
		LastUpdated int64  `bson:"last_updated"`
	}
	err := coll.FindOne(context.Background(), bson.M{"username": username}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Peer{
		ID: doc.ID, Type: storage.PeerType(doc.Type), AccessHash: doc.AccessHash,
		Username: doc.Username, Usernames: doc.Usernames, FirstName: doc.FirstName,
		LastName: doc.LastName, PhoneNumber: doc.PhoneNumber, IsBot: doc.IsBot != 0,
		PhotoID: doc.PhotoID, Language: doc.Language, LastUpdated: doc.LastUpdated,
	}, nil
}

func (m *MongoDB) LoadPeers() ([]*storage.Peer, error) {
	coll := m.db.Collection("peers")
	cursor, err := coll.Find(context.Background(), bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())
	var peers []*storage.Peer
	for cursor.Next(context.Background()) {
		var doc struct {
			ID          int64  `bson:"_id"`
			Type        int    `bson:"type"`
			AccessHash  int64  `bson:"access_hash"`
			Username    string `bson:"username"`
			Usernames   string `bson:"usernames"`
			FirstName   string `bson:"first_name"`
			LastName    string `bson:"last_name"`
			PhoneNumber string `bson:"phone_number"`
			IsBot       int    `bson:"is_bot"`
			PhotoID     int64  `bson:"photo_id"`
			Language    string `bson:"language"`
			LastUpdated int64  `bson:"last_updated"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return nil, err
		}
		peers = append(peers, &storage.Peer{
			ID: doc.ID, Type: storage.PeerType(doc.Type), AccessHash: doc.AccessHash,
			Username: doc.Username, Usernames: doc.Usernames, FirstName: doc.FirstName,
			LastName: doc.LastName, PhoneNumber: doc.PhoneNumber, IsBot: doc.IsBot != 0,
			PhotoID: doc.PhotoID, Language: doc.Language, LastUpdated: doc.LastUpdated,
		})
	}
	return peers, nil
}

func (m *MongoDB) DeletePeer(id int64) error {
	coll := m.db.Collection("peers")
	_, err := coll.DeleteOne(context.Background(), bson.M{"_id": id})
	return err
}

// --- ConversationStore (lazy) ---

func (m *MongoDB) SaveConversation(c *storage.Conversation) error {
	m.ensureConvColl(context.Background())
	coll := m.db.Collection("conversations")
	now := c.UpdatedAt
	if now == 0 {
		now = time.Now().Unix()
	}
	createdAt := c.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	doc := bson.M{
		"name": c.Name, "step": c.Step, "data": c.Data,
		"created_at": createdAt, "updated_at": now,
	}
	filter := bson.M{"chat_id": c.ChatID, "user_id": c.UserID}
	_, err := coll.ReplaceOne(context.Background(), filter, doc, options.Replace().SetUpsert(true))
	if err != nil {
		return fmt.Errorf("mongodb save conversation: %w", err)
	}
	return nil
}

func (m *MongoDB) LoadConversation(chatID, userID int64) (*storage.Conversation, error) {
	m.ensureConvColl(context.Background())
	coll := m.db.Collection("conversations")
	var doc struct {
		ChatID    int64  `bson:"chat_id"`
		UserID    int64  `bson:"user_id"`
		Name      string `bson:"name"`
		Step      int    `bson:"step"`
		Data      []byte `bson:"data"`
		CreatedAt int64  `bson:"created_at"`
		UpdatedAt int64  `bson:"updated_at"`
	}
	err := coll.FindOne(context.Background(), bson.M{"chat_id": chatID, "user_id": userID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &storage.Conversation{
		ChatID: doc.ChatID, UserID: doc.UserID, Name: doc.Name,
		Step: doc.Step, Data: doc.Data, CreatedAt: doc.CreatedAt, UpdatedAt: doc.UpdatedAt,
	}, nil
}

func (m *MongoDB) DeleteConversation(chatID, userID int64) error {
	m.ensureConvColl(context.Background())
	coll := m.db.Collection("conversations")
	_, err := coll.DeleteOne(context.Background(), bson.M{"chat_id": chatID, "user_id": userID})
	return err
}

// --- Close ---

func (m *MongoDB) Close() error {
	return m.client.Disconnect(context.Background())
}

// Package mongodb implements Quarro's first non-SQL provider: MongoDB.
//
// It implements provider.Connector, provider.Discoverer, provider.Browser,
// provider.Aggregator, provider.DocumentEditor, and
// provider.NativeQueryExecutor, never provider.SQLExecutor,
// provider.Editor, provider.Transactor, provider.QueryCompiler, or
// provider.IdentifierFormatter. There is no SQL text to execute or
// compile, so pretending otherwise would be dishonest about what Mongo
// is. What Browse does is honestly a Mongo operation: finding documents
// in a collection with a real Mongo filter, sorted and paged with
// skip/limit, the same shape as browsing a SQL table, but nobody
// pretends filter is a WHERE clause under the hood. Aggregate (see
// aggregate.go) is the same idea for grouped/aggregated charting: it
// translates the Visual Query Builder's model.VisualQuery AST into a real
// $match/$group/$project/$sort/$limit pipeline, for a single collection
// only. Joins are rejected, not faked with $lookup.
//
// Document editing (see edit.go) is whole-document insert/replace/delete
// by _id, deliberately not provider.Editor's flat column-grid model: a
// Mongo document's shape varies document to document and can nest
// arbitrarily, which a fixed-column grid does not fit.
//
// ExecuteNative (see native.go) is this engine's query-document surface:
// a free-form aggregation pipeline the user writes directly, rather than
// one compiled from a Visual Query Builder AST or faked as SQL text. A
// $merge/$out pipeline (a write disguised as a query) is refused before
// it runs.
package mongodb

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/yazeed1s/quarro/internal/connections"
	"github.com/yazeed1s/quarro/internal/database/dialect"
	"github.com/yazeed1s/quarro/internal/database/model"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Provider implements the subset of provider.Provider that MongoDB can
// honestly support. It holds no state; every method takes the connection
// resource it needs as an argument.
type Provider struct{}

// New returns a MongoDB Provider.
func New() Provider { return Provider{} }

// handle is what Open returns as a provider.Handle: a live client already
// bound to the profile's target database, the same way a SQL engine's
// *sql.DB is already bound to one database at connect time. Connection and
// Handle are the same concrete type for Mongo: the driver's *mongo.Client
// is itself a connection pool safe for concurrent use, so there is no
// separate per-statement resource to check out the way database/sql checks
// out a *sql.Conn from a *sql.DB.
type handle struct {
	client *mongo.Client
	db     *mongo.Database
}

// ValidateProfile checks a MongoDB connection profile. Unlike
// dialect.ValidateProfile (what every SQL engine here uses), a username is
// not required: an authless local/dev MongoDB is common, and Mongo has no
// equivalent of a SQL engine's "you must authenticate as someone" model to
// begin with.
func ValidateProfile(p connections.Profile) error {
	if p.Host == "" {
		return fmt.Errorf("host is required")
	}
	if p.Database == "" {
		return fmt.Errorf("database is required")
	}
	if p.Port != "" {
		if n, err := strconv.Atoi(p.Port); err != nil || n < 1 ||
			n > 65535 {
			return fmt.Errorf("port must be between 1 and 65535")
		}
	}
	return nil
}

func (Provider) Open(i connections.Input) (any, error) {
	if err := ValidateProfile(i.Profile); err != nil {
		return nil, err
	}
	port := i.Port
	if port == "" {
		port = "27017"
	}
	uri := &url.URL{Scheme: "mongodb", Host: i.Host + ":" + port}
	if i.Username != "" {
		uri.User = url.UserPassword(i.Username, i.Password)
	}
	opts := options.Client().ApplyURI(uri.String())
	if i.TLSMode != "" && i.TLSMode != "disable" {
		tc, err := dialect.TLSConfig(i.Profile)
		if err != nil {
			return nil, err
		}
		opts.SetTLSConfig(tc)
	}
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("connection failed: %w", err)
	}
	return &handle{
		client: client,
		db:     client.Database(i.Database),
	}, nil
}

func (Provider) OpenConnection(
	_ context.Context,
	h any,
) (any, error) {
	return h.(*handle), nil
}

// DiscardConnection is a no-op: there is no per-connection resource to
// poison here, only the shared client returned by Close.
func (Provider) DiscardConnection(any) error { return nil }

func (Provider) Close(h any) error {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()
	return h.(*handle).client.Disconnect(ctx)
}

func (Provider) Databases(
	ctx context.Context,
	h any,
) ([]string, error) {
	return h.(*handle).client.ListDatabaseNames(ctx, bson.D{})
}

func (Provider) Discover(
	ctx context.Context,
	h any,
) ([]model.Table, error) {
	hd := h.(*handle)
	names, err := hd.db.ListCollectionNames(ctx, bson.D{})
	if err != nil {
		return nil, err
	}
	tables := make([]model.Table, 0, len(names))
	for _, name := range names {
		indexes, err := collectionIndexes(ctx, hd.db.Collection(name))
		if err != nil {
			return nil, err
		}
		tables = append(tables, model.Table{
			Name:        name,
			Schema:      hd.db.Name(),
			ObjectName:  name,
			Kind:        "collection",
			Columns:     []model.Column{},
			Indexes:     indexes,
			ForeignKeys: []model.ForeignKey{},
		})
	}
	return tables, nil
}

func collectionIndexes(
	ctx context.Context,
	coll *mongo.Collection,
) ([]model.Index, error) {
	cur, err := coll.Indexes().List(ctx)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	indexes := []model.Index{}
	for cur.Next(ctx) {
		var spec struct {
			Name   string `bson:"name"`
			Key    bson.D `bson:"key"`
			Unique bool   `bson:"unique"`
		}
		if err := cur.Decode(&spec); err != nil {
			return nil, err
		}
		cols := make([]string, len(spec.Key))
		for i, k := range spec.Key {
			cols[i] = k.Key
		}
		indexes = append(
			indexes,
			model.Index{
				Name:    spec.Name,
				Columns: cols,
				Unique:  spec.Unique,
			},
		)
	}
	return indexes, cur.Err()
}

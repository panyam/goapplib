package gorm

import (
	"context"
	"errors"
	"reflect"
	"testing"

	collv1 "github.com/panyam/goapplib/content/gen/go/collections/v1"
	likesv1 "github.com/panyam/goapplib/content/gen/go/likes/v1"
	tagsv1 "github.com/panyam/goapplib/content/gen/go/tags/v1"
	"github.com/panyam/goapplib/content/services/collections"
	collbackends "github.com/panyam/goapplib/content/services/collections/backends"
	"github.com/panyam/goapplib/content/services/common"
	"github.com/panyam/goapplib/content/services/likes"
	likesbackends "github.com/panyam/goapplib/content/services/likes/backends"
	"github.com/panyam/goapplib/content/services/tags"
	tagsbackends "github.com/panyam/goapplib/content/services/tags/backends"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	alice = "user:alice"
	bob   = "user:bob"
)

// clientAs is a client call made by user, or by an anonymous client when
// user is "".
func clientAs(user string) context.Context {
	ctx := common.WithClientCall(context.Background())
	if user != "" {
		ctx = context.WithValue(ctx, common.DefaultUserIDContextKey, user)
	}
	return ctx
}

func wantCode(t *testing.T, what string, err error, want codes.Code) {
	t.Helper()
	if got := status.Code(err); got != want {
		t.Errorf("%s: code = %v (%v), want %v", what, got, err, want)
	}
}

func newColls(t *testing.T, opts ...collections.ServiceOption) *collbackends.GORMCollectionsService {
	t.Helper()
	svc := collbackends.NewGORMCollectionsService(setupTestDB(t), opts...)
	if err := svc.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	return svc
}

func newTags(t *testing.T, opts ...tags.ServiceOption) *tagsbackends.GORMTagsService {
	t.Helper()
	svc := tagsbackends.NewGORMTagsServiceWithOptions(setupTestDB(t), opts...)
	if err := svc.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	return svc
}

func newLikes(t *testing.T, opts ...likes.ServiceOption) *likesbackends.GORMLikesService {
	t.Helper()
	svc := likesbackends.NewGORMLikesServiceWithOptions(setupTestDB(t), opts...)
	if err := svc.AutoMigrate(); err != nil {
		t.Fatal(err)
	}
	return svc
}

var errHookDenied = errors.New("denied by test hook")

// assertHookRunsFirst calls every method of iface on svc with an empty
// request, and checks that the authorize hook ran first, named for the method,
// and that its denial came back.
func assertHookRunsFirst(t *testing.T, svc any, iface reflect.Type, ops *[]string) {
	t.Helper()
	for i := 0; i < iface.NumMethod(); i++ {
		m := iface.Method(i)
		*ops = nil
		req := reflect.New(m.Type.In(1).Elem())
		out := reflect.ValueOf(svc).MethodByName(m.Name).Call([]reflect.Value{reflect.ValueOf(context.Background()), req})
		err, _ := out[1].Interface().(error)
		if !errors.Is(err, errHookDenied) {
			t.Errorf("%s: err = %v, want the hook's denial", m.Name, err)
		}
		if len(*ops) == 0 || (*ops)[0] != m.Name {
			t.Errorf("%s: hook operations = %v, want %s first", m.Name, *ops, m.Name)
		}
	}
}

func TestAuthorizeHook_RunsFirstOnEveryCollectionsRPC(t *testing.T) {
	var ops []string
	svc := newColls(t, collections.WithOnAuthorize(func(ctx context.Context, hc *collections.HookContext) error {
		ops = append(ops, hc.Operation)
		return errHookDenied
	}))
	assertHookRunsFirst(t, svc, reflect.TypeOf((*collections.CollectionsService)(nil)).Elem(), &ops)
}

func TestAuthorizeHook_RunsFirstOnEveryTagsRPC(t *testing.T) {
	var ops []string
	svc := newTags(t, tags.WithOnAuthorize(func(ctx context.Context, hc *tags.HookContext) error {
		ops = append(ops, hc.Operation)
		return errHookDenied
	}))
	assertHookRunsFirst(t, svc, reflect.TypeOf((*tags.TagsService)(nil)).Elem(), &ops)
}

func TestAuthorizeHook_RunsFirstOnEveryLikesRPC(t *testing.T) {
	var ops []string
	svc := newLikes(t, likes.WithOnAuthorize(func(ctx context.Context, hc *likes.HookContext) error {
		ops = append(ops, hc.Operation)
		return errHookDenied
	}))
	assertHookRunsFirst(t, svc, reflect.TypeOf((*likes.LikesService)(nil)).Elem(), &ops)
}

func TestActingUser_RequestCannotNameAnotherUser(t *testing.T) {
	colls := newColls(t)
	tg := newTags(t)
	lk := newLikes(t)
	coll, err := colls.CreateCollection(context.Background(), &collv1.CreateCollectionRequest{Name: "c", OwnerId: bob})
	if err != nil {
		t.Fatal(err)
	}
	collID := coll.Collection.Id

	writes := map[string]func(ctx context.Context) error{
		"CreateCollection": func(ctx context.Context) error {
			_, err := colls.CreateCollection(ctx, &collv1.CreateCollectionRequest{Name: "x", OwnerId: bob})
			return err
		},
		"AddToCollection": func(ctx context.Context) error {
			_, err := colls.AddToCollection(ctx, &collv1.AddToCollectionRequest{CollectionId: collID, EntityId: "e", AddedBy: bob})
			return err
		},
		"BatchAddToCollection": func(ctx context.Context) error {
			_, err := colls.BatchAddToCollection(ctx, &collv1.BatchAddToCollectionRequest{CollectionId: collID, EntityIds: []string{"e"}, AddedBy: bob})
			return err
		},
		"CreateTag": func(ctx context.Context) error {
			_, err := tg.CreateTag(ctx, &tagsv1.CreateTagRequest{Value: "v", OwnerId: bob})
			return err
		},
		"TagEntity owner": func(ctx context.Context) error {
			_, err := tg.TagEntity(ctx, &tagsv1.TagEntityRequest{EntityId: "e", Value: "v", OwnerId: bob})
			return err
		},
		"TagEntity tagger": func(ctx context.Context) error {
			_, err := tg.TagEntity(ctx, &tagsv1.TagEntityRequest{EntityId: "e", Value: "v", TaggedBy: bob})
			return err
		},
		"BatchTagEntities": func(ctx context.Context) error {
			_, err := tg.BatchTagEntities(ctx, &tagsv1.BatchTagEntitiesRequest{EntityIds: []string{"e"}, Value: "v", OwnerId: bob})
			return err
		},
		"UntagEntity": func(ctx context.Context) error {
			_, err := tg.UntagEntity(ctx, &tagsv1.UntagEntityRequest{EntityId: "e", Value: "v", TaggedBy: bob})
			return err
		},
		"AddReaction": func(ctx context.Context) error {
			_, err := lk.AddReaction(ctx, &likesv1.AddReactionRequest{EntityId: "e", UserId: bob, ReactionType: "like"})
			return err
		},
		"RemoveReaction": func(ctx context.Context) error {
			_, err := lk.RemoveReaction(ctx, &likesv1.RemoveReactionRequest{EntityId: "e", UserId: bob})
			return err
		},
		"ToggleReaction": func(ctx context.Context) error {
			_, err := lk.ToggleReaction(ctx, &likesv1.ToggleReactionRequest{EntityId: "e", UserId: bob, ReactionType: "like"})
			return err
		},
	}
	for name, write := range writes {
		wantCode(t, name+" as alice naming bob", write(clientAs(alice)), codes.PermissionDenied)
		wantCode(t, name+" anonymous naming bob", write(clientAs("")), codes.Unauthenticated)
	}

	// In-process calls with no user in the context keep trusting the request.
	if _, err := lk.AddReaction(context.Background(), &likesv1.AddReactionRequest{EntityId: "e", UserId: bob, ReactionType: "like"}); err != nil {
		t.Errorf("in-process AddReaction naming bob: %v", err)
	}
}

func TestCollectionsOwnerOnly(t *testing.T) {
	svc := newColls(t, collections.WithOnAuthorize(collections.OwnerOnly()))
	asAlice, asBob, anon := clientAs(alice), clientAs(bob), clientAs("")

	create := func(ctx context.Context, name, parent string, vis collv1.CollectionVisibility) *collv1.Collection {
		t.Helper()
		resp, err := svc.CreateCollection(ctx, &collv1.CreateCollectionRequest{Name: name, ParentId: parent, Visibility: vis})
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		return resp.Collection
	}
	private := create(asAlice, "private", "", collv1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE)
	public := create(asAlice, "public", "", collv1.CollectionVisibility_COLLECTION_VISIBILITY_PUBLIC)
	hiddenChild := create(asAlice, "hidden child", public.Id, collv1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE)
	bobs := create(asBob, "bobs", "", collv1.CollectionVisibility_COLLECTION_VISIBILITY_PRIVATE)
	if private.OwnerId != alice {
		t.Fatalf("owner = %q, want %q", private.OwnerId, alice)
	}
	if _, err := svc.AddToCollection(asAlice, &collv1.AddToCollectionRequest{CollectionId: private.Id, EntityId: "e1"}); err != nil {
		t.Fatal(err)
	}

	// Reads of one collection.
	_, err := svc.GetCollection(asBob, &collv1.GetCollectionRequest{Id: private.Id})
	wantCode(t, "bob gets alice's private", err, codes.NotFound)
	_, err = svc.GetCollection(asBob, &collv1.GetCollectionRequest{Id: public.Id})
	wantCode(t, "bob gets alice's public", err, codes.OK)
	_, err = svc.GetCollection(anon, &collv1.GetCollectionRequest{Id: public.Id})
	wantCode(t, "anonymous gets a public", err, codes.OK)
	_, err = svc.GetCollectionItems(asBob, &collv1.GetCollectionItemsRequest{CollectionId: private.Id})
	wantCode(t, "bob lists items of alice's private", err, codes.NotFound)
	_, err = svc.GetCollectionPath(asBob, &collv1.GetCollectionPathRequest{Id: private.Id})
	wantCode(t, "bob gets path of alice's private", err, codes.NotFound)
	_, err = svc.GetCollection(context.Background(), &collv1.GetCollectionRequest{Id: private.Id})
	wantCode(t, "in-process get of a private", err, codes.OK)

	// Writes.
	for _, c := range []struct {
		coll *collv1.Collection
		want codes.Code
	}{{public, codes.PermissionDenied}, {private, codes.NotFound}} {
		id := c.coll.Id
		_, err = svc.UpdateCollection(asBob, &collv1.UpdateCollectionRequest{Collection: &collv1.Collection{Id: id, Name: "pwn"}})
		wantCode(t, "bob updates "+c.coll.Name, err, c.want)
		_, err = svc.DeleteCollection(asBob, &collv1.DeleteCollectionRequest{Id: id, ForceDelete: true})
		wantCode(t, "bob deletes "+c.coll.Name, err, c.want)
		_, err = svc.AddToCollection(asBob, &collv1.AddToCollectionRequest{CollectionId: id, EntityId: "e2"})
		wantCode(t, "bob adds to "+c.coll.Name, err, c.want)
		_, err = svc.RemoveFromCollection(asBob, &collv1.RemoveFromCollectionRequest{CollectionId: id, EntityId: "e1"})
		wantCode(t, "bob removes from "+c.coll.Name, err, c.want)
		_, err = svc.ReorderItems(asBob, &collv1.ReorderItemsRequest{CollectionId: id, ItemOrders: []*collv1.ItemOrder{{EntityId: "e1", DisplayOrder: 9}}})
		wantCode(t, "bob reorders "+c.coll.Name, err, c.want)
		_, err = svc.BatchAddToCollection(asBob, &collv1.BatchAddToCollectionRequest{CollectionId: id, EntityIds: []string{"e2"}})
		wantCode(t, "bob batch-adds to "+c.coll.Name, err, c.want)
		_, err = svc.MoveCollection(asBob, &collv1.MoveCollectionRequest{Id: id})
		wantCode(t, "bob moves "+c.coll.Name, err, c.want)
	}
	_, err = svc.AddToCollection(anon, &collv1.AddToCollectionRequest{CollectionId: public.Id, EntityId: "e2"})
	wantCode(t, "anonymous adds", err, codes.Unauthenticated)
	_, err = svc.DeleteCollection(anon, &collv1.DeleteCollectionRequest{Id: public.Id})
	wantCode(t, "anonymous deletes", err, codes.Unauthenticated)
	_, err = svc.CreateCollection(asBob, &collv1.CreateCollectionRequest{Name: "squat", ParentId: public.Id})
	wantCode(t, "bob creates under alice's public", err, codes.PermissionDenied)
	_, err = svc.MoveCollection(asBob, &collv1.MoveCollectionRequest{Id: bobs.Id, NewParentId: public.Id})
	wantCode(t, "bob moves his under alice's public", err, codes.PermissionDenied)
	_, err = svc.ReorderItems(asAlice, &collv1.ReorderItemsRequest{CollectionId: private.Id, ItemOrders: []*collv1.ItemOrder{{EntityId: "e1", DisplayOrder: 3}}})
	wantCode(t, "alice reorders her own", err, codes.OK)

	// Listing.
	list, err := svc.ListCollections(asBob, &collv1.ListCollectionsRequest{OwnerId: alice})
	wantCode(t, "bob lists alice's", err, codes.OK)
	if got := names(list.GetCollections()); !reflect.DeepEqual(got, []string{"public"}) {
		t.Errorf("bob lists alice's: %v, want only [public]", got)
	}
	list, err = svc.ListCollections(asBob, &collv1.ListCollectionsRequest{})
	wantCode(t, "bob lists his own", err, codes.OK)
	if got := names(list.GetCollections()); !reflect.DeepEqual(got, []string{"bobs"}) {
		t.Errorf("bob lists with no owner: %v, want [bobs]", got)
	}
	_, err = svc.ListCollections(anon, &collv1.ListCollectionsRequest{})
	wantCode(t, "anonymous lists with no owner", err, codes.Unauthenticated)

	_, err = svc.GetEntityCollections(asBob, &collv1.GetEntityCollectionsRequest{EntityId: "e1", OwnerId: alice})
	wantCode(t, "bob reads alice's entity collections", err, codes.PermissionDenied)
	ec, err := svc.GetEntityCollections(asBob, &collv1.GetEntityCollectionsRequest{EntityId: "e1"})
	wantCode(t, "bob reads entity collections", err, codes.OK)
	if n := len(ec.GetCollections()); n != 0 {
		t.Errorf("bob sees %d of alice's collections holding e1, want 0", n)
	}

	// Trees.
	_, err = svc.GetCollectionTree(asBob, &collv1.GetCollectionTreeRequest{})
	wantCode(t, "bob gets the root tree", err, codes.PermissionDenied)
	tree, err := svc.GetCollectionTree(asBob, &collv1.GetCollectionTreeRequest{Id: public.Id})
	wantCode(t, "bob gets alice's public tree", err, codes.OK)
	for _, n := range tree.GetNodes() {
		if n.Collection.Id == hiddenChild.Id {
			t.Error("bob's tree of alice's public collection shows her private child")
		}
	}
}

func names(cs []*collv1.Collection) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Name)
	}
	return out
}

func TestTagsOwnerOnly(t *testing.T) {
	svc := newTags(t, tags.WithOnAuthorize(tags.OwnerOnly()))
	asAlice, asBob, anon := clientAs(alice), clientAs(bob), clientAs("")

	create := func(ctx context.Context, value string, scope tagsv1.TagScope) *tagsv1.Tag {
		t.Helper()
		resp, err := svc.CreateTag(ctx, &tagsv1.CreateTagRequest{Value: value, Scope: scope})
		if err != nil {
			t.Fatalf("create %s: %v", value, err)
		}
		return resp.Tag
	}
	private := create(asAlice, "private", tagsv1.TagScope_TAG_SCOPE_PRIVATE)
	public := create(asAlice, "public", tagsv1.TagScope_TAG_SCOPE_PUBLIC)
	bobs := create(asBob, "bobs", tagsv1.TagScope_TAG_SCOPE_PRIVATE)

	_, err := svc.GetTag(asBob, &tagsv1.GetTagRequest{Id: private.Id})
	wantCode(t, "bob gets alice's private tag", err, codes.NotFound)
	_, err = svc.GetTag(anon, &tagsv1.GetTagRequest{Id: public.Id})
	wantCode(t, "anonymous gets a public tag", err, codes.OK)

	for _, c := range []struct {
		tag  *tagsv1.Tag
		want codes.Code
	}{{public, codes.PermissionDenied}, {private, codes.NotFound}} {
		_, err = svc.UpdateTag(asBob, &tagsv1.UpdateTagRequest{Tag: &tagsv1.Tag{Id: c.tag.Id, Color: "red"}})
		wantCode(t, "bob updates "+c.tag.Value, err, c.want)
		_, err = svc.DeleteTag(asBob, &tagsv1.DeleteTagRequest{Id: c.tag.Id})
		wantCode(t, "bob deletes "+c.tag.Value, err, c.want)
	}
	_, err = svc.UpdateTag(asAlice, &tagsv1.UpdateTagRequest{Tag: &tagsv1.Tag{Id: private.Id, Color: "red"}})
	wantCode(t, "alice updates her tag", err, codes.OK)

	_, err = svc.TagEntity(asBob, &tagsv1.TagEntityRequest{EntityId: "e", TagId: private.Id})
	wantCode(t, "bob applies alice's private tag", err, codes.NotFound)
	_, err = svc.TagEntity(asBob, &tagsv1.TagEntityRequest{EntityId: "e", TagId: public.Id})
	wantCode(t, "bob applies alice's public tag", err, codes.OK)
	_, err = svc.TagEntity(anon, &tagsv1.TagEntityRequest{EntityId: "e", Value: "x"})
	wantCode(t, "anonymous tags", err, codes.Unauthenticated)

	_, err = svc.MergeTags(asBob, &tagsv1.MergeTagsRequest{SourceTagId: bobs.Id, TargetTagId: public.Id})
	wantCode(t, "bob merges into alice's tag", err, codes.PermissionDenied)
	_, err = svc.PromoteTag(asBob, &tagsv1.PromoteTagRequest{SourceTagId: bobs.Id, TargetOwnerId: "system"})
	wantCode(t, "bob promotes a tag", err, codes.PermissionDenied)

	list, err := svc.ListTags(asBob, &tagsv1.ListTagsRequest{OwnerId: alice})
	wantCode(t, "bob lists alice's tags", err, codes.OK)
	for _, tag := range list.GetTags() {
		if tag.Scope != tagsv1.TagScope_TAG_SCOPE_PUBLIC {
			t.Errorf("bob's list of alice's tags includes %q (%v)", tag.Value, tag.Scope)
		}
	}
	_, err = svc.GetEntityTags(asBob, &tagsv1.GetEntityTagsRequest{EntityId: "e", OwnerId: alice})
	wantCode(t, "bob reads alice's entity tags", err, codes.PermissionDenied)
	_, err = svc.GetEntitiesWithTag(asBob, &tagsv1.GetEntitiesWithTagRequest{TagId: private.Id})
	wantCode(t, "bob lists entities with alice's private tag", err, codes.NotFound)
	_, err = svc.SearchTags(asBob, &tagsv1.SearchTagsRequest{Query: "p", OwnerId: alice})
	wantCode(t, "bob searches alice's tags", err, codes.PermissionDenied)
}

func TestLikesOwnerOnly(t *testing.T) {
	svc := newLikes(t, likes.WithOnAuthorize(likes.OwnerOnly()))
	asBob, anon := clientAs(bob), clientAs("")

	resp, err := svc.ToggleReaction(asBob, &likesv1.ToggleReactionRequest{EntityId: "e", ReactionType: "like"})
	wantCode(t, "bob toggles", err, codes.OK)
	if got := resp.GetLike().GetUserId(); got != bob {
		t.Errorf("like user = %q, want %q", got, bob)
	}
	_, err = svc.ToggleReaction(anon, &likesv1.ToggleReactionRequest{EntityId: "e", ReactionType: "like"})
	wantCode(t, "anonymous toggles", err, codes.Unauthenticated)
	_, err = svc.GetLikeCounts(anon, &likesv1.GetLikeCountsRequest{EntityId: "e"})
	wantCode(t, "anonymous reads counts", err, codes.OK)
	_, err = svc.ListUserReactions(asBob, &likesv1.ListUserReactionsRequest{UserId: alice})
	wantCode(t, "bob lists alice's reactions", err, codes.PermissionDenied)
	_, err = svc.BatchGetUserReactions(asBob, &likesv1.BatchGetUserReactionsRequest{UserId: alice, EntityIds: []string{"e"}})
	wantCode(t, "bob batch-reads alice's reactions", err, codes.PermissionDenied)
	own, err := svc.ListUserReactions(asBob, &likesv1.ListUserReactionsRequest{})
	wantCode(t, "bob lists his reactions", err, codes.OK)
	if n := len(own.GetLikes()); n != 1 {
		t.Errorf("bob has %d reactions, want 1", n)
	}
	_, err = svc.CreateReactionType(asBob, &likesv1.CreateReactionTypeRequest{ReactionType: &likesv1.ReactionType{Id: "pwn"}})
	wantCode(t, "bob creates a reaction type", err, codes.PermissionDenied)
	_, err = svc.CreateReactionType(context.Background(), &likesv1.CreateReactionTypeRequest{ReactionType: &likesv1.ReactionType{Id: "clap"}})
	wantCode(t, "in-process reaction type", err, codes.OK)
}

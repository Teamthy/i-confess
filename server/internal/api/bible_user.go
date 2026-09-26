package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Teamthy/i-confess/internal/bible"
	"github.com/Teamthy/i-confess/internal/httpx"
	"github.com/google/uuid"
)

type bibleMarkInput struct {
	TranslationID string `json:"translation_id"`
	BookID string `json:"book_id"`
	Chapter int `json:"chapter"`
	Verse int `json:"verse"`
	Color string `json:"color"`
	Label string `json:"label"`
	Note string `json:"note"`
}

func (h *Handler) validateBibleMark(r *http.Request, in bibleMarkInput) (bible.Verse, bool) {
	if in.TranslationID=="" || in.Chapter<1 || in.Chapter>200 || in.Verse<1 || in.Verse>300 { return bible.Verse{},false }
	if _,ok:=bible.ParseBook(in.BookID);!ok{return bible.Verse{},false}
	if err:=h.readableBibleTranslation(r.Context(),in.TranslationID);err!=nil{return bible.Verse{},false}
	verse,err:=h.bible.GetVerse(r.Context(),in.TranslationID,in.BookID,in.Chapter,in.Verse)
	return verse,err==nil
}

func (h *Handler) listMyBibleBookmarks(w http.ResponseWriter,r *http.Request){
	rows,err:=h.db.QueryContext(r.Context(),`SELECT id,translation_id,book_id,chapter,verse,COALESCE(label,''),COALESCE(note,''),created_at,updated_at,row_version FROM verse_bookmarks WHERE user_id=? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 500`,h.userID(r))
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your saved passages.");return};defer rows.Close();items:=[]map[string]any{}
	for rows.Next(){var id,translation,book,label,note string;var chapter,verse,version int;var created,updated string;if err:=rows.Scan(&id,&translation,&book,&chapter,&verse,&label,&note,&created,&updated,&version);err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your saved passages.");return};items=append(items,map[string]any{"id":id,"translation_id":translation,"book_id":book,"chapter":chapter,"verse":verse,"reference":bible.CanonicalVerseID(book,chapter,verse),"label":label,"note":note,"created_at":created,"updated_at":updated,"row_version":version})}
	if err:=rows.Err();err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your saved passages.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"bookmarks":items})
}
func (h *Handler) saveMyBibleBookmark(w http.ResponseWriter,r *http.Request){
	var in bibleMarkInput;if err:=httpx.DecodeJSON(r,&in);err!=nil{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_BOOKMARK","Invalid bookmark.");return}
	if len(in.Label)>120||len(in.Note)>4000{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_BOOKMARK","Bookmark details are too long.");return}
	verse,ok:=h.validateBibleMark(r,in);if !ok{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_REFERENCE","Choose an available Bible verse.");return}
	id:=uuid.NewString();now:=time.Now().UTC();var savedID string;var version int
	err:=h.db.QueryRowContext(r.Context(),`INSERT INTO verse_bookmarks(id,user_id,version_id,book_id,chapter,verse,label,note,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL DO UPDATE SET label=EXCLUDED.label,note=EXCLUDED.note,updated_at=EXCLUDED.updated_at,row_version=verse_bookmarks.row_version+1 RETURNING id,row_version`,id,h.userID(r),in.TranslationID,verse.BookID,in.Chapter,in.Verse,strings.TrimSpace(in.Label),strings.TrimSpace(in.Note),now.Format(time.RFC3339Nano),now.Format(time.RFC3339Nano)).Scan(&savedID,&version)
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to save this bookmark.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"id":savedID,"translation_id":in.TranslationID,"book_id":verse.BookID,"chapter":in.Chapter,"verse":in.Verse,"reference":verse.ID,"label":strings.TrimSpace(in.Label),"note":strings.TrimSpace(in.Note),"row_version":version})
}
func (h *Handler) deleteMyBibleBookmark(w http.ResponseWriter,r *http.Request){
	result,err:=h.db.ExecContext(r.Context(),`UPDATE verse_bookmarks SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,time.Now().UTC().Format(time.RFC3339Nano),time.Now().UTC().Format(time.RFC3339Nano),r.PathValue("id"),h.userID(r))
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to remove this bookmark.");return};n,_:=result.RowsAffected();if n==0{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Bookmark not found.");return};w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listMyBibleHighlights(w http.ResponseWriter,r *http.Request){
	rows,err:=h.db.QueryContext(r.Context(),`SELECT id,version_id,book_id,chapter,verse,COALESCE(color,''),created_at,updated_at,row_version FROM verse_highlights WHERE user_id=? AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1000`,h.userID(r))
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your highlights.");return};defer rows.Close();items:=[]map[string]any{}
	for rows.Next(){var id,translation,book,color,created,updated string;var chapter,verse,version int;if err:=rows.Scan(&id,&translation,&book,&chapter,&verse,&color,&created,&updated,&version);err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your highlights.");return};items=append(items,map[string]any{"id":id,"translation_id":translation,"book_id":book,"chapter":chapter,"verse":verse,"reference":bible.CanonicalVerseID(book,chapter,verse),"color":color,"created_at":created,"updated_at":updated,"row_version":version})}
	if err:=rows.Err();err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your highlights.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"highlights":items})
}
func (h *Handler) saveMyBibleHighlight(w http.ResponseWriter,r *http.Request){
	var in bibleMarkInput;if err:=httpx.DecodeJSON(r,&in);err!=nil{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_HIGHLIGHT","Invalid highlight.");return}
	allowed:=map[string]bool{"yellow":true,"green":true,"blue":true,"pink":true,"purple":true};if !allowed[in.Color]{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_HIGHLIGHT","Choose a valid highlight color.");return}
	verse,ok:=h.validateBibleMark(r,in);if !ok{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_REFERENCE","Choose an available Bible verse.");return}
	id:=uuid.NewString();now:=time.Now().UTC().Format(time.RFC3339Nano);var savedID string;var version int
	err:=h.db.QueryRowContext(r.Context(),`INSERT INTO verse_highlights(id,user_id,version_id,book_id,chapter,verse,color,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,1) ON CONFLICT(user_id,version_id,book_id,chapter,verse) WHERE deleted_at IS NULL DO UPDATE SET color=EXCLUDED.color,updated_at=EXCLUDED.updated_at,row_version=verse_highlights.row_version+1 RETURNING id,row_version`,id,h.userID(r),in.TranslationID,verse.BookID,in.Chapter,in.Verse,in.Color,now,now).Scan(&savedID,&version)
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to save this highlight.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"id":savedID,"translation_id":in.TranslationID,"book_id":verse.BookID,"chapter":in.Chapter,"verse":in.Verse,"reference":verse.ID,"color":in.Color,"row_version":version})
}
func (h *Handler) deleteMyBibleHighlight(w http.ResponseWriter,r *http.Request){
	now:=time.Now().UTC().Format(time.RFC3339Nano);result,err:=h.db.ExecContext(r.Context(),`UPDATE verse_highlights SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,now,now,r.PathValue("id"),h.userID(r));if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to remove this highlight.");return};n,_:=result.RowsAffected();if n==0{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Highlight not found.");return};w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) listMyBibleNotes(w http.ResponseWriter,r *http.Request){
	rows,err:=h.db.QueryContext(r.Context(),`SELECT id,book_id,chapter,verse_start,verse_end,COALESCE(translation_id,''),COALESCE(title,''),body,created_at,updated_at,row_version FROM user_bible_notes WHERE user_id=? AND deleted_at IS NULL ORDER BY updated_at DESC LIMIT 500`,h.userID(r));if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your notes.");return};defer rows.Close();items:=[]map[string]any{}
	for rows.Next(){var id,book,translation,title,body,created,updated string;var chapter,start,end,version int;if err:=rows.Scan(&id,&book,&chapter,&start,&end,&translation,&title,&body,&created,&updated,&version);err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your notes.");return};items=append(items,map[string]any{"id":id,"book_id":book,"chapter":chapter,"verse_start":start,"verse_end":end,"translation_id":translation,"title":title,"body":body,"created_at":created,"updated_at":updated,"row_version":version})}
	if err:=rows.Err();err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your notes.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"notes":items})
}
func validateNoteReference(book string,chapter,start,end int)bool{bookID,ok:=bible.ParseBook(book);if !ok||chapter<1||start<1||end<start||end-start>199{return false};b,_:=bible.BookByID(bookID);return chapter<=b.Chapters()&&end<=b.Verses[chapter-1]}
func (h *Handler) createMyBibleNote(w http.ResponseWriter,r *http.Request){
	var in struct{BookID string `json:"book_id"`;Chapter int `json:"chapter"`;VerseStart int `json:"verse_start"`;VerseEnd int `json:"verse_end"`;TranslationID string `json:"translation_id"`;Title string `json:"title"`;Body string `json:"body"`;DeviceID string `json:"device_id"`}
	if err:=httpx.DecodeJSON(r,&in);err!=nil||!validateNoteReference(in.BookID,in.Chapter,in.VerseStart,in.VerseEnd)||strings.TrimSpace(in.Body)==""||len(in.Body)>20000||len(in.Title)>200{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_NOTE","Enter a valid reference and a note up to 20,000 characters.");return}
	if strings.TrimSpace(in.TranslationID)!="" { if err:=h.readableBibleTranslation(r.Context(),in.TranslationID);err!=nil{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_TRANSLATION","Choose an available Bible translation.");return} }
	bookID,_:=bible.ParseBook(in.BookID);id:=uuid.NewString();now:=time.Now().UTC().Format(time.RFC3339Nano);var created,updated time.Time
	err:=h.db.QueryRowContext(r.Context(),`INSERT INTO user_bible_notes(id,user_id,book_id,chapter,verse_start,verse_end,translation_id,title,body,device_id,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1) RETURNING created_at,updated_at`,id,h.userID(r),bookID,in.Chapter,in.VerseStart,in.VerseEnd,nullIfBlank(in.TranslationID),strings.TrimSpace(in.Title),strings.TrimSpace(in.Body),strings.TrimSpace(in.DeviceID),now,now).Scan(&created,&updated)
	if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to save your note.");return};httpx.WriteJSON(w,http.StatusCreated,map[string]any{"id":id,"book_id":bookID,"chapter":in.Chapter,"verse_start":in.VerseStart,"verse_end":in.VerseEnd,"translation_id":in.TranslationID,"title":strings.TrimSpace(in.Title),"body":strings.TrimSpace(in.Body),"created_at":created,"updated_at":updated,"row_version":1})
}
func nullIfBlank(s string)any{if strings.TrimSpace(s)==""{return nil};return strings.TrimSpace(s)}
func (h *Handler) patchMyBibleNote(w http.ResponseWriter,r *http.Request){
	var in struct{Title *string `json:"title"`;Body *string `json:"body"`;RowVersion *int `json:"row_version"`}
	if err:=httpx.DecodeJSON(r,&in);err!=nil||in.Title==nil&&in.Body==nil{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_NOTE","Provide a note title or body to update.");return}
	if in.Title!=nil&&len(*in.Title)>200||in.Body!=nil&&(len(*in.Body)>20000||strings.TrimSpace(*in.Body)==""){writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_NOTE","Note fields exceed the allowed size.");return}
	title:=any(nil);body:=any(nil);if in.Title!=nil{title=strings.TrimSpace(*in.Title)};if in.Body!=nil{body=strings.TrimSpace(*in.Body)};expected:=0;if in.RowVersion!=nil{expected=*in.RowVersion}
	var noteID,book,storedTitle,storedBody string;var chapter,start,end,version int;var translation sql.NullString;var created,updated time.Time
	err:=h.db.QueryRowContext(r.Context(),`UPDATE user_bible_notes SET title=COALESCE(?,title),body=COALESCE(?,body),updated_at=now(),row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL AND (?=0 OR row_version=?) RETURNING id,book_id,chapter,verse_start,verse_end,translation_id,COALESCE(title,''),body,created_at,updated_at,row_version`,title,body,r.PathValue("id"),h.userID(r),expected,expected).Scan(&noteID,&book,&chapter,&start,&end,&translation,&storedTitle,&storedBody,&created,&updated,&version)
	if errors.Is(err,sql.ErrNoRows){writeCode(w,http.StatusConflict,"BIBLE_NOTE_CONFLICT","This note changed elsewhere. Reload it before editing.");return};if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to update your note.");return};translationID:="";if translation.Valid{translationID=translation.String};httpx.WriteJSON(w,http.StatusOK,map[string]any{"id":noteID,"book_id":book,"chapter":chapter,"verse_start":start,"verse_end":end,"translation_id":translationID,"title":storedTitle,"body":storedBody,"created_at":created,"updated_at":updated,"row_version":version})
}
func (h *Handler) deleteMyBibleNote(w http.ResponseWriter,r *http.Request){now:=time.Now().UTC().Format(time.RFC3339Nano);result,err:=h.db.ExecContext(r.Context(),`UPDATE user_bible_notes SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,now,now,r.PathValue("id"),h.userID(r));if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to delete your note.");return};n,_:=result.RowsAffected();if n==0{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Note not found.");return};w.WriteHeader(http.StatusNoContent)
}


// Private Bible collections store canonical references, never copied verse text.
func (h *Handler) listMyBibleCollections(w http.ResponseWriter,r *http.Request){
 rows,err:=h.db.QueryContext(r.Context(),`SELECT c.id,c.name,c.description,c.created_at,c.updated_at,c.row_version,i.id,i.translation_id,i.book_id,i.chapter,i.verse,i.created_at FROM user_bible_collections c LEFT JOIN user_bible_collection_items i ON i.collection_id=c.id AND i.user_id=c.user_id AND i.deleted_at IS NULL WHERE c.user_id=? AND c.deleted_at IS NULL ORDER BY c.updated_at DESC,i.created_at`,h.userID(r))
 if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your Bible collections.");return};defer rows.Close()
 byID:=map[string]map[string]any{};ordered:=make([]map[string]any,0)
 for rows.Next(){var id,name,description string;var created,updated time.Time;var version int;var itemID,book sql.NullString;var translation sql.NullString;var chapter,verse sql.NullInt64;var itemCreated sql.NullTime
  if err:=rows.Scan(&id,&name,&description,&created,&updated,&version,&itemID,&translation,&book,&chapter,&verse,&itemCreated);err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your Bible collections.");return}
  collection:=byID[id];if collection==nil{collection=map[string]any{"id":id,"name":name,"description":description,"created_at":created,"updated_at":updated,"row_version":version,"items":[]map[string]any{}};byID[id]=collection;ordered=append(ordered,collection)}
  if itemID.Valid{translationID:="";if translation.Valid{translationID=translation.String};items:=collection["items"].([]map[string]any);items=append(items,map[string]any{"id":itemID.String,"translation_id":translationID,"book_id":book.String,"chapter":chapter.Int64,"verse":verse.Int64,"created_at":itemCreated.Time});collection["items"]=items}
 }
 if err:=rows.Err();err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your Bible collections.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"collections":ordered})
}

func (h *Handler) createMyBibleCollection(w http.ResponseWriter,r *http.Request){
	var in struct{Name string `json:"name"`;Description string `json:"description"`}
	if err:=httpx.DecodeJSON(r,&in);err!=nil||strings.TrimSpace(in.Name)==""||len(strings.TrimSpace(in.Name))>80||len(strings.TrimSpace(in.Description))>500{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_COLLECTION","Enter a collection name up to 80 characters and description up to 500 characters.");return}
	id:=uuid.NewString();now:=time.Now().UTC();_,err:=h.db.ExecContext(r.Context(),`INSERT INTO user_bible_collections(id,user_id,name,description,created_at,updated_at,row_version) VALUES(?,?,?,?,?,?,1)`,id,h.userID(r),strings.TrimSpace(in.Name),strings.TrimSpace(in.Description),now,now);if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to create your Bible collection.");return};httpx.WriteJSON(w,http.StatusCreated,map[string]any{"id":id,"name":strings.TrimSpace(in.Name),"description":strings.TrimSpace(in.Description),"created_at":now,"updated_at":now,"row_version":1,"items":[]any{}})
}

func (h *Handler) patchMyBibleCollection(w http.ResponseWriter,r *http.Request){
	var in struct{Name *string `json:"name"`;Description *string `json:"description"`;RowVersion int `json:"row_version"`}
	if err:=httpx.DecodeJSON(r,&in);err!=nil||in.Name==nil&&in.Description==nil||in.Name!=nil&&(strings.TrimSpace(*in.Name)==""||len(strings.TrimSpace(*in.Name))>80)||in.Description!=nil&&len(strings.TrimSpace(*in.Description))>500{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_COLLECTION","Provide a valid collection name or description.");return}
	name,description:=any(nil),any(nil);if in.Name!=nil{name=strings.TrimSpace(*in.Name)};if in.Description!=nil{description=strings.TrimSpace(*in.Description)};var id,storedName,storedDescription string;var version int;var updated time.Time
	err:=h.db.QueryRowContext(r.Context(),`UPDATE user_bible_collections SET name=COALESCE(?,name),description=COALESCE(?,description),updated_at=now(),row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL AND (?=0 OR row_version=?) RETURNING id,name,description,updated_at,row_version`,name,description,r.PathValue("id"),h.userID(r),in.RowVersion,in.RowVersion).Scan(&id,&storedName,&storedDescription,&updated,&version)
	if errors.Is(err,sql.ErrNoRows){writeCode(w,http.StatusConflict,"BIBLE_COLLECTION_CONFLICT","This collection changed elsewhere or is no longer available.");return};if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to update your Bible collection.");return};httpx.WriteJSON(w,http.StatusOK,map[string]any{"id":id,"name":storedName,"description":storedDescription,"updated_at":updated,"row_version":version})
}

func (h *Handler) deleteMyBibleCollection(w http.ResponseWriter,r *http.Request){
 now:=time.Now().UTC();tx,err:=h.db.BeginTx(r.Context(),nil);if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to delete your Bible collection.");return};defer tx.Rollback()
 result,err:=tx.ExecContext(r.Context(),`UPDATE user_bible_collections SET deleted_at=?,updated_at=?,row_version=row_version+1 WHERE id=? AND user_id=? AND deleted_at IS NULL`,now,now,r.PathValue("id"),h.userID(r));if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to delete your Bible collection.");return};n,_:=result.RowsAffected();if n==0{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Collection not found.");return}
 if _,err=tx.ExecContext(r.Context(),`UPDATE user_bible_collection_items SET deleted_at=? WHERE collection_id=? AND user_id=? AND deleted_at IS NULL`,now,r.PathValue("id"),h.userID(r));err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to delete private collection references.");return};if err=tx.Commit();err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to complete collection deletion.");return};w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) addMyBibleCollectionItem(w http.ResponseWriter,r *http.Request){
	var in bibleMarkInput;if err:=httpx.DecodeJSON(r,&in);err!=nil{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_REFERENCE","Choose a reviewed verse to save.");return};verse,ok:=h.validateBibleMark(r,in);if !ok{writeCode(w,http.StatusBadRequest,"BIBLE_INVALID_REFERENCE","Choose an available Bible verse.");return}
	var exists bool;if err:=h.db.QueryRowContext(r.Context(),`SELECT EXISTS(SELECT 1 FROM user_bible_collections WHERE id=? AND user_id=? AND deleted_at IS NULL)`,r.PathValue("id"),h.userID(r)).Scan(&exists);err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to load your Bible collection.");return};if !exists{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Collection not found.");return}
	id:=uuid.NewString();now:=time.Now().UTC();err:=h.db.QueryRowContext(r.Context(),`INSERT INTO user_bible_collection_items(id,collection_id,user_id,translation_id,book_id,chapter,verse,created_at) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(collection_id,book_id,chapter,verse) WHERE deleted_at IS NULL DO NOTHING RETURNING id`,id,r.PathValue("id"),h.userID(r),in.TranslationID,verse.BookID,in.Chapter,in.Verse,now).Scan(&id)
	if errors.Is(err,sql.ErrNoRows){err=h.db.QueryRowContext(r.Context(),`SELECT id FROM user_bible_collection_items WHERE collection_id=? AND user_id=? AND book_id=? AND chapter=? AND verse=? AND deleted_at IS NULL`,r.PathValue("id"),h.userID(r),verse.BookID,in.Chapter,in.Verse).Scan(&id)};if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to save this collection reference.");return};httpx.WriteJSON(w,http.StatusCreated,map[string]any{"id":id,"collection_id":r.PathValue("id"),"translation_id":in.TranslationID,"book_id":verse.BookID,"chapter":in.Chapter,"verse":in.Verse,"created_at":now})
}

func (h *Handler) deleteMyBibleCollectionItem(w http.ResponseWriter,r *http.Request){
	result,err:=h.db.ExecContext(r.Context(),`UPDATE user_bible_collection_items SET deleted_at=? WHERE id=? AND collection_id=? AND user_id=? AND deleted_at IS NULL`,time.Now().UTC(),r.PathValue("itemID"),r.PathValue("id"),h.userID(r));if err!=nil{httpx.WriteError(w,http.StatusInternalServerError,"Unable to remove this collection reference.");return};n,_:=result.RowsAffected();if n==0{writeCode(w,http.StatusNotFound,"BIBLE_NOT_FOUND","Collection reference not found.");return};w.WriteHeader(http.StatusNoContent)
}

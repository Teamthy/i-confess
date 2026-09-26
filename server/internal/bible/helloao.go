package bible

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const helloAODefaultBaseURL = "https://bible.helloao.org/api"
const maxHelloAOResponse = 12 << 20

type helloAOTranslationWire struct {
	ID string `json:"id"`; Name string `json:"name"`; EnglishName string `json:"englishName"`
	Website string `json:"website"`; LicenseURL string `json:"licenseUrl"`; ShortName string `json:"shortName"`
	Language string `json:"language"`; LanguageName string `json:"languageName"`; LanguageEnglishName string `json:"languageEnglishName"`
	TextDirection string `json:"textDirection"`; NumberOfBooks int `json:"numberOfBooks"`
	TotalNumberOfChapters int `json:"totalNumberOfChapters"`; TotalNumberOfVerses int `json:"totalNumberOfVerses"`
}

type helloAOBookWire struct {
	ID string `json:"id"`; Name string `json:"name"`; CommonName string `json:"commonName"`
	Order int `json:"order"`; NumberOfChapters int `json:"numberOfChapters"`
}

type helloAOChapterWire struct {
	Translation helloAOTranslationWire `json:"translation"`
	Book helloAOBookWire `json:"book"`
	Chapter struct { Number int `json:"number"`; Content []struct { Type string `json:"type"`; Number int `json:"number"`; Text string `json:"text"` } `json:"content"` } `json:"chapter"`
}

// HelloAOBibleProvider is a server-side adapter for HelloAO's documented JSON
// API. It intentionally does not infer redistribution rights from API access or
// from a license URL: newly discovered translations are pending review and all
// use-specific rights are default-deny until an operator verifies the terms.
type HelloAOBibleProvider struct {
	baseURL string
	client *http.Client
	mu sync.Mutex
	failures int
	openUntil time.Time
	translations []Translation
	translationsExpiry time.Time
	books map[string][]BookInfo
	booksExpiry map[string]time.Time
	chapters map[string]cachedChapter
}

type cachedChapter struct { value Chapter; expires time.Time }

func NewHelloAOBibleProvider(baseURL string, client *http.Client) (*HelloAOBibleProvider, error) {
	if strings.TrimSpace(baseURL) == "" { baseURL = helloAODefaultBaseURL }
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, fmt.Errorf("bible provider: invalid HelloAO base URL")
	}
	if u.Scheme != "https" && !strings.EqualFold(u.Hostname(), "localhost") && !strings.HasPrefix(u.Hostname(), "127.") {
		return nil, fmt.Errorf("bible provider: HTTPS is required for remote HelloAO endpoints")
	}
	if client == nil { client = &http.Client{Timeout: 8 * time.Second} }
	return &HelloAOBibleProvider{baseURL: strings.TrimRight(baseURL, "/"), client: client,
		books: map[string][]BookInfo{}, booksExpiry: map[string]time.Time{}, chapters: map[string]cachedChapter{}}, nil
}

func (p *HelloAOBibleProvider) get(ctx context.Context, path string, dst any) error {
	p.mu.Lock()
	if time.Now().Before(p.openUntil) { p.mu.Unlock(); return ErrUnavailable }
	p.mu.Unlock()
	endpoint := p.baseURL + "/" + strings.TrimLeft(path, "/")
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil { return ErrUnavailable }
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "iCONFESS-BibleService/1.0 (+https://iconfess.com)")
		resp, err := p.client.Do(req)
		if err != nil {
			last = err
		} else {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxHelloAOResponse+1))
			_ = resp.Body.Close()
			if readErr != nil { last = readErr
			} else if len(body) > maxHelloAOResponse { last = fmt.Errorf("provider response exceeds size limit")
			} else if resp.StatusCode == http.StatusNotFound { p.recordSuccess(); return ErrNotFound
			} else if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
				last = fmt.Errorf("provider returned retryable status %d", resp.StatusCode)
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				p.recordFailure(); return ErrUnavailable
			} else if err := json.Unmarshal(body, dst); err != nil {
				p.recordFailure(); return ErrUnavailable
			} else { p.recordSuccess(); return nil }
		}
		if attempt < 2 {
			t := time.NewTimer(time.Duration(attempt+1) * 100 * time.Millisecond)
			select { case <-ctx.Done(): t.Stop(); return ErrUnavailable; case <-t.C: }
		}
	}
	p.recordFailure()
	_ = last // Provider details stay in server-side telemetry, not user errors.
	return ErrUnavailable
}

func (p *HelloAOBibleProvider) recordSuccess() { p.mu.Lock(); p.failures = 0; p.openUntil = time.Time{}; p.mu.Unlock() }
func (p *HelloAOBibleProvider) recordFailure() { p.mu.Lock(); p.failures++; if p.failures >= 3 { p.openUntil = time.Now().Add(20*time.Second) }; p.mu.Unlock() }

func (p *HelloAOBibleProvider) GetTranslations(ctx context.Context, language, query string) ([]Translation, error) {
	p.mu.Lock(); cached := append([]Translation(nil), p.translations...); fresh := time.Now().Before(p.translationsExpiry); p.mu.Unlock()
	if !fresh {
		var wire struct { Translations []helloAOTranslationWire `json:"translations"` }
		if err := p.get(ctx, "available_translations.json", &wire); err != nil { return nil, err }
		cached = make([]Translation, 0, len(wire.Translations))
		for _, item := range wire.Translations { cached = append(cached, normalizeHelloAOTranslation(item)) }
		p.mu.Lock(); p.translations = append([]Translation(nil), cached...); p.translationsExpiry = time.Now().Add(10*time.Minute); p.mu.Unlock()
	}
	language = strings.ToLower(strings.TrimSpace(language)); query = strings.ToLower(strings.TrimSpace(query))
	out := make([]Translation, 0, len(cached))
	for _, t := range cached {
		if language != "" && !strings.EqualFold(language, t.Language) && !strings.EqualFold(language, t.LanguageName) { continue }
		if query != "" && !strings.Contains(strings.ToLower(t.Name+" "+t.Abbreviation+" "+t.LanguageName), query) { continue }
		out = append(out, t)
	}
	return out, nil
}

func normalizeHelloAOTranslation(w helloAOTranslationWire) Translation {
	id := strings.TrimSpace(w.ID)
	name := w.EnglishName; if name == "" { name = w.Name }
	languageName := w.LanguageEnglishName; if languageName == "" { languageName = w.LanguageName }
	direction := strings.ToLower(w.TextDirection); if direction != "rtl" { direction = "ltr" }
	license := "Unreviewed — consult provider license terms"
	return Translation{ID: strings.ToLower(id), Provider: "helloao", ProviderTranslationID: id, Name: name,
		Abbreviation: w.ShortName, Language: helloAOLanguageCode(w.Language), LanguageName: languageName,
		Direction: direction, License: license, LicenseURL: w.LicenseURL, SourceURL: w.Website,
		AttributionRequired: true, Status: "pending_review", BookCount: w.NumberOfBooks,
		ChapterCount: w.TotalNumberOfChapters, VerseCount: w.TotalNumberOfVerses}
}

func helloAOLanguageCode(code string) string {
	known := map[string]string{"eng":"en","spa":"es","por":"pt","fra":"fr","fre":"fr","ita":"it","swa":"sw","tgl":"tl","arb":"ar","ara":"ar","heb":"he","yor":"yo","ibo":"ig","hau":"ha","amh":"am","zul":"zu","xho":"xh","deu":"de","ger":"de","nld":"nl","dut":"nl"}
	if v := known[strings.ToLower(code)]; v != "" { return v }; return strings.ToLower(code)
}

func (p *HelloAOBibleProvider) GetTranslation(ctx context.Context, id string) (Translation, error) {
	items, err := p.GetTranslations(ctx, "", ""); if err != nil { return Translation{}, err }
	for _, item := range items { if strings.EqualFold(item.ID,id) || strings.EqualFold(item.ProviderTranslationID,id) { return item,nil } }
	return Translation{}, ErrNotFound
}
func (p *HelloAOBibleProvider) GetMetadata(ctx context.Context,id string) (Translation,error) { return p.GetTranslation(ctx,id) }

func (p *HelloAOBibleProvider) GetBooks(ctx context.Context, translationID string) ([]BookInfo,error) {
	key:=strings.ToUpper(translationID); p.mu.Lock(); cached:=append([]BookInfo(nil),p.books[key]...); fresh:=time.Now().Before(p.booksExpiry[key]); p.mu.Unlock()
	if fresh { return cached,nil }
	var wire struct { Books []helloAOBookWire `json:"books"` }
	if err:=p.get(ctx,url.PathEscape(key)+"/books.json",&wire); err!=nil{return nil,err}
	out:=make([]BookInfo,0,len(wire.Books))
	for _,b:=range wire.Books { id,ok:=ParseBook(b.ID); if !ok { id,ok=ParseBook(b.CommonName) }; if !ok { continue }; canonical,ok:=BookByID(id); if !ok { continue }; name:=b.CommonName; if name=="" {name=b.Name}; chapters:=b.NumberOfChapters; if chapters<1 {chapters=canonical.Chapters()}; out=append(out,BookInfo{ID:id,Name:name,Testament:canonical.Testament,CanonicalOrder:b.Order,ChapterCount:chapters}) }
	p.mu.Lock(); p.books[key]=append([]BookInfo(nil),out...); p.booksExpiry[key]=time.Now().Add(10*time.Minute); p.mu.Unlock(); return out,nil
}
func (p *HelloAOBibleProvider) GetBook(ctx context.Context, translationID, bookID string)(BookInfo,error){books,err:=p.GetBooks(ctx,translationID);if err!=nil{return BookInfo{},err};canon,ok:=ParseBook(bookID);if !ok{return BookInfo{},ErrNotFound};for _,b:=range books{if b.ID==canon{return b,nil}};return BookInfo{},ErrNotFound}

func (p *HelloAOBibleProvider) GetChapter(ctx context.Context, translationID, bookID string, chapterNumber int)(Chapter,error){
	canonID,ok:=ParseBook(bookID);if !ok||chapterNumber<1{return Chapter{},ErrNotFound};
	cacheKey:=strings.ToUpper(translationID)+":"+canonID+":"+strconv.Itoa(chapterNumber)
	p.mu.Lock();cached,exists:=p.chapters[cacheKey];p.mu.Unlock();if exists&&time.Now().Before(cached.expires){return cached.value,nil}
	book,err:=p.GetBook(ctx,translationID,canonID);if err!=nil{return Chapter{},err};
	var wire helloAOChapterWire
	path:=url.PathEscape(strings.ToUpper(translationID))+"/"+url.PathEscape(usfmCodeFor(canonID))+"/"+strconv.Itoa(chapterNumber)+".simple.json"
	if err:=p.get(ctx,path,&wire);err!=nil{return Chapter{},err}
	translation,err:=p.GetTranslation(ctx,translationID);if err!=nil{return Chapter{},err}
	verses:=make([]Verse,0,len(wire.Chapter.Content));for _,part:=range wire.Chapter.Content{if part.Type!="verse"||part.Number<1{continue};verses=append(verses,Verse{ID:CanonicalVerseID(canonID,chapterNumber,part.Number),Number:part.Number,Text:part.Text,BookID:canonID,Chapter:chapterNumber})}
	if len(verses)==0{return Chapter{},ErrNotFound};out:=Chapter{Translation:translation,Book:book,Chapter:chapterNumber,Verses:verses};p.mu.Lock();p.chapters[cacheKey]=cachedChapter{value:out,expires:time.Now().Add(5*time.Minute)};p.mu.Unlock();return out,nil
}
func usfmCodeFor(osis string)string{for code,id:=range usfm{if id==osis{return code}};return ""}
func (p *HelloAOBibleProvider) GetVerse(ctx context.Context,t,b string,c,v int)(Verse,error){chapter,err:=p.GetChapter(ctx,t,b,c);if err!=nil{return Verse{},err};for _,verse:=range chapter.Verses{if verse.Number==v{return verse,nil}};return Verse{},ErrNotFound}
func (p *HelloAOBibleProvider) GetPassage(ctx context.Context,t,raw string)(Passage,error){ref,err:=ParseReference(raw);if err!=nil{return Passage{},err};translation,err:=p.GetTranslation(ctx,t);if err!=nil{return Passage{},err};chapter,err:=p.GetChapter(ctx,t,ref.Book,ref.Chapter);if err!=nil{return Passage{},err};verses:=chapter.Verses;if ref.StartVerse>0{verses=nil;for _,v:=range chapter.Verses{if v.Number>=ref.StartVerse&&v.Number<=ref.EndVerse{verses=append(verses,v)}}};return Passage{Reference:strings.TrimSpace(raw),Translation:translation,Verses:verses},nil}
func (p *HelloAOBibleProvider) GetLanguages(ctx context.Context)([]Language,error){translations,err:=p.GetTranslations(ctx,"","");if err!=nil{return nil,err};seen:=map[string]Language{};for _,t:=range translations{if t.Language==""{continue};if _,ok:=seen[t.Language];!ok{seen[t.Language]=Language{ID:t.Language,ISO6393:t.Language,BCP47:t.Language,Name:t.LanguageName,NativeName:t.LanguageName,Direction:t.Direction,Status:"active"}}};out:=make([]Language,0,len(seen));for _,v:=range seen{out=append(out,v)};return out,nil}
func (p *HelloAOBibleProvider) Search(context.Context,string,string,string,int)([]SearchResult,error){return nil,ErrUnavailable}
func (p *HelloAOBibleProvider) GetCrossReferences(context.Context,string)([]string,error){return nil,ErrUnavailable}
func (p *HelloAOBibleProvider) HealthCheck(ctx context.Context)error{var wire struct{Translations []helloAOTranslationWire `json:"translations"`};return p.get(ctx,"available_translations.json",&wire)}

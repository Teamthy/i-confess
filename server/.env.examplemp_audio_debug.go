package main
import (
  "context"
  "database/sql"
  "fmt"
  _ "modernc.org/sqlite"
  "github.com/Teamthy/i-confess/internal/db"
  "github.com/Teamthy/i-confess/internal/models"
  "github.com/Teamthy/i-confess/internal/store"
)
func main(){
  conn, err := sql.Open("sqlite", ":memory:")
  if err != nil { panic(err) }
  if err := db.InitSchema(conn, db.SchemaSQL); err != nil { panic(err) }
  _, err = conn.Exec("INSERT INTO voices (id,name,type,language,premium,status,created_at,updated_at) VALUES ('v1','test','professional','en',0,'active','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')")
  if err != nil { panic(err) }
  _, err = conn.Exec("INSERT INTO categories (id,name,slug,status,created_at,updated_at) VALUES ('c1','cat','cat','published','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')")
  if err != nil { panic(err) }
  _, err = conn.Exec("INSERT INTO confessions (id,category_id,title,language,status,created_at,updated_at) VALUES ('conf1','c1','title','en','published','2024-01-01T00:00:00Z','2024-01-01T00:00:00Z')")
  if err != nil { panic(err) }
  a := &models.AudioAsset{ID:"a1",ConfessionID:"conf1",VoiceID:"v1",URL:"https://example.com/x.mp3",DurationSeconds:30,SizeBytes:123,Status:"ready"}
  err = store.NewAudioStore(conn).UpsertAsset(context.Background(), a)
  fmt.Printf("ERR=%v\n", err)
  if err == nil { fmt.Printf("saved=%+v\n", a) }
}

package main
import ("fmt";"log";"net/http";"os";"github.com/stockyard-dev/stockyard-telegraph/internal/server";"github.com/stockyard-dev/stockyard-telegraph/internal/store")
func main(){port:=os.Getenv("PORT");if port==""{port="9030"};dataDir:=os.Getenv("DATA_DIR");if dataDir==""{dataDir="./telegraph-data"}
db,err:=store.Open(dataDir);if err!=nil{log.Fatalf("telegraph: %v",err)};defer db.Close();srv:=server.New(db)
fmt.Printf("\n  Telegraph — notification hub\n  Dashboard:  http://localhost:%s/ui\n  API:        http://localhost:%s/api\n\n",port,port)
log.Printf("telegraph: listening on :%s",port);log.Fatal(http.ListenAndServe(":"+port,srv))}

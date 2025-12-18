package main

import (
    _ "embed"
    "os"
	"html/template"
    "fmt"
	"log"
    "net/http"
    "encoding/json"
    "context"
    "strconv"
	"github.com/gorilla/sessions"
    "github.com/morganmwalker/go-spire-api-client"
)

//go:embed customers.json
var settingsFile []byte

type App struct {
    Client *spireclient.SpireClient
    Store  *sessions.CookieStore
}

// Define the name of the session, used to load/save the data
const sessionName = "spire-session" 

// Define the keys to store the credentials within the session
const usernameKey = "spireUsername"
const passwordKey = "spirePassword"

// Define the context key
type key string
const agentKey key = "spireAgent"

var tmpl *template.Template

func init() {
    tmpl = template.Must(template.ParseGlob("templates/*.html"))
}

func (a *App) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
	}

	username := r.FormValue("username")
	password := r.FormValue("password")

    if username == "" || password == "" {
        log.Printf("Login failed: empty credentials submitted.")
        http.Redirect(w, r, "/", http.StatusSeeOther) 
        return
    }

    agent := spireclient.SpireAgent{ 
        Username: username,
        Password: password,
    }

	if err := a.Client.ValidateSpireCredentials(agent); err != nil {
		log.Printf("Login failed for user %v. Validation Error: %v", username, err)
		http.Redirect(w, r, "/", http.StatusSeeOther) 
        return
	}

	session, err := a.Store.Get(r, sessionName)
	if err != nil {
        log.Printf("Session error: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
        return		
	}

	session.Values[usernameKey] = username
	session.Values[passwordKey] = password

    if err = session.Save(r, w); err != nil {
        log.Printf("Failed to save session: %v", err)
        http.Error(w, "Failed to save session", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/home", http.StatusSeeOther)
}

func (a *App) loginHandler(w http.ResponseWriter, r *http.Request) {
    session, _ := a.Store.Get(r, sessionName)
    if _, ok := session.Values[usernameKey]; ok {
        http.Redirect(w, r, "/home", http.StatusSeeOther)
        return
    }

    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    err := tmpl.ExecuteTemplate(w, "login.html", nil)
    if err != nil {
        http.Error(w, "Error loading login page", http.StatusInternalServerError)
    }
}

func (a *App) logoutHandler(w http.ResponseWriter, r *http.Request) {
    session, err := a.Store.Get(r, sessionName)
    if err != nil {
        http.Redirect(w, r, "/", http.StatusSeeOther)
        return
    }

	session.Values[usernameKey] = nil
	session.Values[passwordKey] = nil

    // Delete cookies immediately
    session.Options.MaxAge = -1

    if err = session.Save(r, w); err != nil {
        http.Error(w, "Failed to save session", http.StatusInternalServerError)
        return
    }

    http.Redirect(w, r, "/", http.StatusSeeOther)    
}

func (a *App) authMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        session, err := a.Store.Get(r, sessionName)
        if err != nil {
            http.Redirect(w, r, "/", http.StatusSeeOther)
            return
        }

        usernameVal, usernameOK := session.Values[usernameKey]
        passwordVal, passwordOK := session.Values[passwordKey]
        
        if !usernameOK || !passwordOK {
            http.Redirect(w, r, "/", http.StatusSeeOther)
            return
        }
        
        agent := spireclient.SpireAgent{
            Username: usernameVal.(string),
            Password: passwordVal.(string),
        }
        
        ctx := context.WithValue(r.Context(), agentKey, agent)

        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func GetSpireAgent(r *http.Request) spireclient.SpireAgent {
    if agent, ok := r.Context().Value(agentKey).(spireclient.SpireAgent); ok {
        return agent
    }
    return spireclient.SpireAgent{} 
}

type OrderDetails struct {
    OrderNo string `json:"orderNo"`
    PurchaseNo string `json:"purchaseNo"`
}

// Gets all sales items associated with the provided map of orders
func (a *App) getOrderItems(orders map[string]OrderDetails, agent spireclient.SpireAgent) ([]map[string]interface{}, error) {
	// Make a filter for an HTTP request that gets the items for every order submitted
	// Should look like:
	// { "$or": [ { "orderNo": orderNo1 }, { "orderNo": orderNo2}, ... ] }
	noOrders := len(orders)

	orConditions := make([]map[string]string, 0, noOrders)

	for _, order := range orders {
		condition := map[string]string{"orderNo": order.OrderNo}
		orConditions = append(orConditions, condition)
	}

	itemFilter := map[string]interface{}{"$or": orConditions}

    items, err := a.Client.FetchSpireData("/sales/items", itemFilter, agent)
	if err != nil {
		return nil, err
	}
	return items, nil
}

type SubmitPayload struct {
	CustomerNo string `json:"customerNo"` // The expected customer number for all orders
	Orders map[string]OrderDetails `json:"orders"` // Map of Order ID -> Order No
}

func (a *App) submitSelectedOrdersHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    agent := GetSpireAgent(r)
    
    var receivedPayload SubmitPayload
    err := json.NewDecoder(r.Body).Decode(&receivedPayload)
    if err != nil {
        log.Printf("Error decoding JSON: %v", err)
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    log.Printf("Received %d selected order IDs for processing:", len(receivedPayload.Orders))

    var toDelete []string
    purchaseNoMap := make(map[string]string)
    for id, order := range receivedPayload.Orders {
        toDelete = append(toDelete, id)
        purchaseNoMap[order.OrderNo] = order.PurchaseNo
        log.Printf("Order No: %v  Purchase No: %v", order.OrderNo, order.PurchaseNo)
    }

    items, err := a.getOrderItems(receivedPayload.Orders, agent)

    if err != nil {
        log.Printf("Error getting submitted orders: %v", err)
        return
    }

    itemMap := createItemMap(items)
    log.Printf("Payload: %v", itemMap)

    log.Printf("CustomerNo: %v", receivedPayload.CustomerNo)

    submitPayload, err := buildPayload(itemMap, receivedPayload.CustomerNo, purchaseNoMap)
    if err != nil {
        log.Printf("Error creating payload: %v", err)
        return
    }
    jsonData, err := json.MarshalIndent(submitPayload, "", "  ")
    if err != nil {
        log.Printf("Error marshalling payload for log: %v", err)
    } else {
        log.Printf("Payload to submit (JSON):\n%s", jsonData)
    }
    log.Printf("Payload to submit: %v", submitPayload)

    response, err := a.Client.CreateSalesOrder(agent, submitPayload)
    if err != nil {
        log.Printf("Error creating sales order: %v", err)
        return
    }
    log.Printf("Response: %v", response)

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]interface{}{
        "status": "success", 
        "message": "Order successfully created in Spire. Ready to delete source orders.",
        "toDelete": toDelete,
    })
}

// Loops through a list of sales order IDs and tries to delete the orders in Spire
func (a *App) deleteSalesOrders(orderList []string, agent spireclient.SpireAgent) error {
	for _, orderID := range orderList {
        _, err := a.Client.SpireRequest("/sales/orders/"+orderID, agent, "DELETE", nil)
		if err != nil {
			return fmt.Errorf("failed to delete order %s: %v", orderID, err)
		}
	}
	return nil
}

func (a *App) deleteOrdersHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
        return
    }

    agent := GetSpireAgent(r)
    
    var payload struct {
        OrderIDs []string `json:"orderIDs"`
    }
    
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    log.Printf("Attempting to delete %d source orders.", len(payload.OrderIDs))
    
    if err := a.deleteSalesOrders(payload.OrderIDs, agent); err != nil {
        log.Printf("Error deleting sales orders: %v", err)
        w.WriteHeader(http.StatusInternalServerError)
        json.NewEncoder(w).Encode(map[string]string{"status": "error", "message": fmt.Sprintf("Failed to delete all source orders: %v", err.Error())})
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"status": "success", "message": "Source orders deleted successfully."})
}

// Given a list of items, creates a map of that holds the order number as the key and the list of corresponding items as the value
func createItemMap(orderItems []map[string]interface{}) map[string][]interface{} {
    itemMap := make(map[string][]interface{}, len(orderItems))

    for _, item := range orderItems {
        orderNo, ok := item["orderNo"].(string)
        if !ok {
            log.Printf("%v passed as order no, expecting a string", item["orderNo"])
            continue
        }
        list := itemMap[orderNo]
        itemMap[orderNo] = append(list, item)
    }
    return itemMap
}

type CustomerSettings struct {
    RequiresPO bool   `json:"requires_po"`
    DiscountRate float64 `json:"discount_rate"`
}

func loadSettingsFromFile() (map[string]CustomerSettings, error) {
    settingsMap := make(map[string]CustomerSettings)

    if err := json.Unmarshal(settingsFile, &settingsMap); err != nil {
        return nil, fmt.Errorf("failed to unmarshal embedded customer settings: %v", err)
    }

    return settingsMap, nil
}

// Helper to build the comment and enforce PO requirements
func buildOrderComment(orderNo string, customerNo string, purchaseNoMap map[string]string, setting CustomerSettings) (string, error) {    
    comment := "Packing Slip Number - " + orderNo
    if setting.RequiresPO {
        poNo, poExists := purchaseNoMap[orderNo]
        if !poExists || poNo == "" {
            return "", fmt.Errorf("customer %s requires a PO, but none was found for order %s", customerNo, orderNo)
        }
        comment = comment + " PO " + poNo
    }
    return comment, nil
}

// Helper to process a single item, including validation and price calculation
func processItem(itemI interface{}, customerSetting CustomerSettings) (map[string]interface{}, error) {
    // Type assertion for the item
    item, ok := itemI.(map[string]interface{})
    if !ok {
        return nil, fmt.Errorf("expecting item of type map[string]interface{}")
    }

    // Only include items of type 1 and 2
    itemType, typeOK := item["itemType"].(float64)
    if !typeOK || (itemType != 1 && itemType != 2) {
        return nil, fmt.Errorf("skipped item of itemType %v", itemType)
    }

    // Price conversion and calculation
    unitPrice, priceOK := item["unitPrice"].(string) 
    if !priceOK {
        return nil, fmt.Errorf("item %v skipped: invalid or missing unitPrice", item["partNo"])
    }
    unitPriceFloat, err := strconv.ParseFloat(unitPrice, 64) 
    if err != nil {
        return nil, fmt.Errorf("item %v skipped: type conversion failed for unitPrice: %v", item["partNo"], err)
    }

    retailPrice := unitPriceFloat / (1.0 - (customerSetting.DiscountRate / 100))
    retailPriceStr := fmt.Sprintf("%.2f", retailPrice)

    // Build the final item map
    return map[string]interface{}{
        "whse": item["whse"],
        "partNo": item["partNo"],
        "description": item["description"],
        "orderQty": item["orderQty"],
        "committedQty": item["committedQty"],
        "sellMeasure": item["sellMeasure"],
        "retailPrice": retailPriceStr,
        "discountPct": customerSetting.DiscountRate,
    }, nil
}

// Builds payload to submit to Spire
func buildPayload(itemMap map[string][]interface{}, customerNo string, purchaseNoMap map[string]string) (map[string]interface{}, error) {
    settingsMap, err := loadSettingsFromFile()
    if err != nil {
        log.Printf("Failed to load customer settings: %v", err)
        return nil, err
    }

    // Look up customer settings from the returned map
    customerSetting, ok := settingsMap[customerNo]
    if !ok {
        log.Printf("Customer not found, using default settings")
        customerSetting = CustomerSettings{RequiresPO: false, DiscountRate: 0.0}
    }

    var itemsData []interface{} 

    // Loop through the item map to build the item data for the payload
    for orderNo, itemList := range itemMap {
        comment, err := buildOrderComment(orderNo, customerNo, purchaseNoMap, customerSetting)
        if err != nil {
            return nil, err
        }
        itemsData = append(itemsData, map[string]interface{}{
            "comment": comment,
        })
        for _, itemI := range itemList {
            itemPayload, err := processItem(itemI, customerSetting)
            if err != nil {
                log.Println(err.Error())
                continue 
            }
            itemsData = append(itemsData, itemPayload)
        }
    }

    payload := map[string]interface{}{
        "customer": map[string]string{
            "customerNo": customerNo,
        },
        "items": itemsData,
    }
    return payload, nil
}

func (a *App) homeHandler(w http.ResponseWriter, r *http.Request) {
    agent := GetSpireAgent(r)

    salesOrderFilter := map[string]interface{}{"type": "O"}
    records, err := a.Client.FetchSpireData("/sales/orders", salesOrderFilter, agent)

    if err != nil {
        log.Printf("Error: %v", err)
    }
      
    w.Header().Set("Content-Type", "text/html; charset=utf-8")
    err = tmpl.ExecuteTemplate(w, "home.html", records)
    if err != nil{
        http.Error(w, "Error loading homepage", http.StatusInternalServerError)
    }
}

func main() {
    rootURL := os.Getenv("SPIRE_ROOT_URL")
    if rootURL == "" {
        log.Fatal("SPIRE_ROOT_URL environment variable is not set.")
    }
    log.Printf("Using: %v", rootURL)

    key := os.Getenv("SECRET_KEY")
    sessionStore := sessions.NewCookieStore([]byte(key))

    // Initialize client instance
    spireClientInstance := spireclient.NewSpireClient(rootURL)

    // Initialize app dependancies
    app := &App{
        Client: spireClientInstance,
        Store: sessionStore,
    }

    router := http.NewServeMux()

    fileServer := http.FileServer(http.Dir("./static"))
    router.Handle("/static/", http.StripPrefix("/static", fileServer))

    router.HandleFunc("/", app.loginHandler)
    router.HandleFunc("/login", app.loginSubmit)
    router.HandleFunc("/logout", app.logoutHandler)

    // Apply middleware
    router.Handle("/home", app.authMiddleware(http.HandlerFunc(app.homeHandler)))
    router.Handle("/submit_selected_orders", app.authMiddleware(http.HandlerFunc(app.submitSelectedOrdersHandler)))
    router.Handle("/delete_source_orders", app.authMiddleware(http.HandlerFunc(app.deleteOrdersHandler)))

    log.Println("Server listening on :8080")
    if err := http.ListenAndServe(":8080", router); err != nil {
        log.Fatalf("Server failed to start: %v", err)
    }
}

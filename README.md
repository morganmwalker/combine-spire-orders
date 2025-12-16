# combine-sales-orders

Full-stack web application desgined to combine Spire Systems sales orders, retaining information about the original orders, including the order number and PO number, as comments.

Works with Spire's API to allow selection from Spire's open sales orders and the creation of a new, consolidated sales order, all from the browser. Requires Go installation, as well as access to Spire Systems API. 

## Installation

Clone or download the repository. In your terminal, run `go mod tidy` to resolve dependances. Configure your environment variables, `SPIRE_ROOT_URL` (in the form `https://{spire-url}:10880/api/v2/companies/{company}`) and `SECRET_KEY` (for session encryption).

## Run
In the terminal, run `go run main.go` and navigate to `http://localhost:8080` in the browser.

## Using the application
Log in with your Spire credentials. Upon landing on the homepage, all open sales orders are fetch from Spire and are made available for selection. Select the sales orders you wish to combine and hit the "Submit" button at the bottom of the page. A successful request will prompt you to delete the selected source orders.

To add customer specific requirements, add a file named `customers.json` to the project directory, using the following format (using your actual customer number):
```json
{
    "SpireCustomerNo": {
        "requires_po":   true,
        "discount_rate": 20.0
    },
    ...
}
```
For customers not found in the file, the default is `"requires_po": false` and `"discount_rate": 0.0`.

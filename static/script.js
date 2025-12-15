// Submit button
let submitButton = document.getElementById("submitButton");

// Array to hold the selected orders for later submission
var selectedOrdersData = [];

// Filter function for customer prefix
function filterList() {
    var input = document.getElementById("customerPrefix").value.toLowerCase();
    var orderList = document.getElementById("orderList");
    var items = orderList.getElementsByTagName("li");

    for (var i = 0; i < items.length; i++) {
        listItem = items[i]
        var orderId = listItem.getAttribute("data-order-id");
        var customerNo = listItem.getAttribute("data-customer-no").toLowerCase();
        const isSelected = selectedOrdersData.some(order => order.orderId === orderId);

        if (customerNo.startsWith(input) && !isSelected) {
            listItem.style.display = "";
        } else {
            listItem.style.display = "none";
        }
    }
}

function setSubmitButtonState() {
    if (selectedOrdersData.length > 0) {
        submitButton.disabled = false;
    }
    else {
        submitButton.disabled = true;
    }
}

function addAllOpenOrders() {
    const orderList = document.getElementById("orderList");
    const selectedOrdersList = document.getElementById("selectedOrders");
    
    if (!orderList || !selectedOrdersList) {
        console.error("Order list containers not found.");
        return;
    }

    const openListItems = orderList.querySelectorAll("li");

    openListItems.forEach(listItem => {
        if (listItem.style.display !== "none") {

            const orderId = listItem.getAttribute("data-order-id");
            const isAlreadySelected = selectedOrdersList.querySelector(`li[data-order-id="${orderId}"]`);

            if (!isAlreadySelected) {
                const selectButton = listItem.querySelector('button[onclick="addToSelected(this)"]');
                
                if (selectButton) {
                    addToSelected(selectButton);
                }
            }
        }
    });
}

function addToSelected(button) {
    var listItem = button.parentElement;
    var orderId = listItem.getAttribute("data-order-id");
    var orderNo = listItem.getAttribute("data-order-no");
    var customerNo = listItem.getAttribute("data-customer-no");
    var purchaseNo = listItem.getAttribute("data-po-no");

    const isAlreadyInArray = selectedOrdersData.some(order => order.orderId === orderId);

    if (isAlreadyInArray) {
        // If the order is already in the array, stop execution here.
        // This is a safety net against erroneous calls.
        console.warn(`Order ${orderNo} (ID: ${orderId}) is already selected. Skipping addition.`);
        return; 
    }

    var selectedItem = document.createElement("li");
    selectedItem.setAttribute("data-order-id", orderId);
    selectedItem.setAttribute("data-order-no", orderNo);
    selectedItem.setAttribute("data-customer-no", customerNo);
    selectedItem.setAttribute("data-po-no", purchaseNo);
    selectedItem.innerHTML = `
        <span class="order-data">
            <strong>Order:</strong> ${orderNo}
        </span>
        <span class="customer-data">
            <strong>Customer:</strong> ${customerNo}
        </span>
        <button type="button" onclick="removeFromSelected(this)">Deselect</button>
    `;

    document.getElementById("selectedOrders").appendChild(selectedItem);

    selectedOrdersData.push({ orderId, orderNo, customerNo, purchaseNo });

    listItem.style.display = "none";

    setSubmitButtonState()
}

// Remove an order from the selected orders list (deselect)
function removeFromSelected(button) {
    var selectedItem = button.parentElement;
    var orderId = selectedItem.getAttribute("data-order-id");
    var orderNo = selectedItem.getAttribute("data-order-no");
    var customerNo = selectedItem.getAttribute("data-customer-no");
    var purchaseNo = selectedItem.getAttribute("data-po-no");

    // Remove the order from the selectedOrdersData array
    selectedOrdersData = selectedOrdersData.filter(order => order.orderId !== orderId);
    // Remove the order from the displayed selected orders list
    selectedItem.remove();

    // Re-enable the "Select" button and show the order again in the original list
    var orderList = document.getElementById("orderList");
    var orderItems = orderList.getElementsByTagName("li");

    // Loop through the list and find the corresponding order, then show it again
    for (var i = 0; i < orderItems.length; i++) {
        if (orderItems[i].getAttribute("data-order-id") === orderId) {
            orderItems[i].style.display = ""; // Show the order again
            break;
        }
    }

    setSubmitButtonState()
}

function showDeleteConfirmation(count) {
    return new Promise((resolve) => {
        const modal = document.getElementById("deleteModal");
        const confirmBtn = document.getElementById("confirmDeleteBtn");
        const cancelBtn = document.getElementById("cancelDeleteBtn");
        
        document.getElementById("orderCount").textContent = count;
        modal.style.display = "block";
        //document.getElementById('deleteModal').classList.add('is-visible');

        const handleConfirm = () => {
            confirmBtn.removeEventListener('click', handleConfirm);
            cancelBtn.removeEventListener('click', handleCancel);
            modal.style.display = "none";
            //document.getElementById('deleteModal').classList.remove('is-visible');
            resolve(true); // Deletion confirmed
        };

        const handleCancel = () => {
            // Clean up listeners
            confirmBtn.removeEventListener('click', handleConfirm);
            cancelBtn.removeEventListener('click', handleCancel);
            modal.style.display = "none";
            resolve(false); // Deletion cancelled
        };

        confirmBtn.addEventListener('click', handleConfirm);
        cancelBtn.addEventListener('click', handleCancel);
    });
}

async function deleteOrders(orderIDs) {
    if (orderIDs.length === 0) {
        return;
    }
    
    const confirmation = await showDeleteConfirmation(orderIDs.length);

    if (confirmation) {
        displayMessage("Deleting source orders...");
        
        const payload = { orderIDs: orderIDs };

        fetch('/delete_source_orders', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify(payload),
        })
        .then(response => {
            if (response.status === 200) {
                return response.json();
            }
            // Handle errors from the deletion endpoint
            return response.text().then(text => {throw new Error(text)});
        })
        .then(data => {
            // Success response from deletion endpoint
            displayMessage(data.message);
            // Optionally: Reload the page to refresh the Open Sales Orders list
            // setTimeout(() => location.reload(), 2000); 
        })
        .catch(error => {
            // Deletion failed (e.g., API error during delete)
            displayMessage("Error during deletion: " + error.message, true);
        });
    } else {
        displayMessage("Combined order submitted. Source orders NOT deleted.");
    }
}

// Handle form submission to send selected orders to the backend
function submitSelectedOrders() {
    // Compare each order's customer number to the first selected order
    const requiredCustomerNo = selectedOrdersData[0].customerNo;
    const allMatch = selectedOrdersData.every(order => order.customerNo === requiredCustomerNo);

    if (!allMatch) {
        alert("All selected orders must belong to the same customer! (" + requiredCustomerNo + ").");
        return;
    }
    const orders = selectedOrdersData.reduce((map, order) => {
        map[order.orderId] = {
            orderNo: order.orderNo,
            purchaseNo: order.purchaseNo // Include purchaseNo here
        };
        return map;
    }, {});

    const payload = {
        customerNo: requiredCustomerNo,
        orders: orders
    };

    displayMessage("Submitting orders to Spire...");

    fetch('/submit_selected_orders', {
        method: 'POST',
        headers: {
            'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
    })
    .then(response =>  {
        if (response.status == 200) {
            return response.json();
        }
        return response.text().then(text => {throw new Error(text)});
    })
    .then(data => {
        displayMessage("Orders submitted successfully!");
        // Clear selected order
        document.getElementById("selectedOrders").innerHTML = '';
        selectedOrdersData = [];
        // Delete the selected orders from Spire
        deleteOrders(data.toDelete);
    })
    .catch(error => {
        displayMessage("Error submitting orders: " + error.message, true);
    });
}

// Add a helper function to manage the message area
function displayMessage(message, isError = false) {
    const messageArea = document.getElementById("messageArea");
    messageArea.textContent = message;
    
    if (message) {
        // Show the box when there is a message
        messageArea.style.display = "block"; 
        // Optional: Change colors for errors
        if (isError) {
            messageArea.style.backgroundColor = '#fce4e4'; // Light red
            messageArea.style.borderColor = '#f44336'; // Red border
            messageArea.style.color = '#c62828'; // Dark red text
        } else {
            messageArea.style.backgroundColor = '#e8f5e9'; // Green background
            messageArea.style.borderColor = 'var(--accent-color)'; // Green border
            messageArea.style.color = '#1b5e20'; // Green text
        }
    } else {
        // Hide the box when the message is empty
        messageArea.style.display = "none";
    }
}
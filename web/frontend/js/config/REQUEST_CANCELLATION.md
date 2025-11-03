# Request Cancellation System

Automatic HTTP request cancellation system on page navigation.

## How it works

1. **RequestCancellationManager** - manages `AbortController` for each route
2. **Axios Interceptor** - automatically adds `signal` to each request
3. **Router Guard** - cancels previous page requests on navigation

## Automatic usage

All requests through `axios` are automatically cancelled on page navigation. No additional actions required.

### Store action example

```javascript
import axios from '../config/axios'

async fetchData() {
    this.apiProcesses++
    try {
        const response = await axios.get('/api/data')
        this.data = response.data
    } catch (error) {
        // Ignore cancelled requests
        if (error.__CANCEL__) {
            return
        }
        throw error
    } finally {
        this.apiProcesses--
    }
}
```

## Manual usage in components

If you need to cancel requests on component unmount (not on page navigation):

```javascript
import { useRequestCancellation } from '../composables/useRequestCancellation'
import axios from '../config/axios'

export default {
    setup() {
        const { signal, abort } = useRequestCancellation()

        const fetchData = async () => {
            try {
                // Explicitly pass signal for cancellation on component unmount
                const response = await axios.get('/api/data', { signal })
                return response.data
            } catch (error) {
                if (error.__CANCEL__) {
                    console.log('Request cancelled')
                    return
                }
                throw error
            }
        }

        // Can cancel manually
        const cancelRequests = () => {
            abort()
        }

        return {
            fetchData,
            cancelRequests
        }
    }
}
```

## Disabling automatic cancellation for specific request

If you need to disable automatic cancellation for a specific request:

```javascript
// Pass null to signal
const response = await axios.get('/api/important-data', {
    signal: null
})
```

## Cancelling all active requests

```javascript
import { requestCancellation } from '../config/requestCancellation'

// Cancel all requests from all pages
requestCancellation.abortAllRequests()

// Cancel requests for specific page
requestCancellation.abortRequests('/admin/servers')
```

## Handling cancellation errors

When a request is cancelled, axios throws an error with `__CANCEL__` flag. In store actions you need to check this flag:

```javascript
try {
    await axios.get('/api/data')
} catch (error) {
    if (error.__CANCEL__) {
        // Request cancelled - don't show error to user
        return
    }
    // Regular error - show to user
    showError(error)
}
```

## Important notes

1. Request cancellation doesn't guarantee that server will stop processing
2. POST/PUT/DELETE requests are also cancelled - be careful with mutating operations
3. For critical operations (payment, deletion) consider using `signal: null`

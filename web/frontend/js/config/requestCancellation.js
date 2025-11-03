class RequestCancellationManager {
    constructor() {
        this.controllers = new Map()
        this.currentRoute = null
    }

    createController(routePath) {
        if (!this.controllers.has(routePath)) {
            this.controllers.set(routePath, new AbortController())
        }
        return this.controllers.get(routePath)
    }

    getController(routePath) {
        return this.controllers.get(routePath)
    }

    abortRequests(routePath) {
        const controller = this.controllers.get(routePath)
        if (controller) {
            controller.abort()
            this.controllers.delete(routePath)
        }
    }

    abortAllRequests() {
        this.controllers.forEach((controller, routePath) => {
            controller.abort()
        })
        this.controllers.clear()
    }

    setCurrentRoute(routePath) {
        if (this.currentRoute && this.currentRoute !== routePath) {
            this.abortRequests(this.currentRoute)
        }
        this.currentRoute = routePath
    }

    getCurrentController() {
        if (!this.currentRoute) {
            return null
        }
        return this.getController(this.currentRoute)
    }
}

export const requestCancellation = new RequestCancellationManager()

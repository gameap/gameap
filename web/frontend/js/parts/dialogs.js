import {trans} from "@/i18n/i18n";

const errorNotification = function(error, callback) {
    if (error.__CANCEL__) {
        return
    }

    let title = trans('main.error')
    let content = ''

    switch (typeof error) {
        case "string":
            content = error
            break
        case "object":
            ({title, content} = parseErrorObject(error))
            break
        default:
            content = "Something went wrong."
    }

    notification({
        title: title,
        content: content,
        type: 'error'
    })
}

const parseErrorObject = function(error) {
    const result = {
        title: trans('main.error'),
        content: "Something went wrong.",
    }

    if ('message' in error) {
        result.content = error.message
    }

    if ('response' in error) {
        if ('statusText' in error.response) {
            result.title = error.response.statusText
        }

        if ('data' in error.response && error.response.data && typeof error.response.data === 'object' && 'message' in error.response.data) {
            result.content = error.response.data.message
        }
    } else {
        if ('title' in error) {
            result.title = error.title
        } else if ('name' in error) {
            result.title = error.name
        }

        if ('content' in error) {
            result.content = error.content
        } else if ('message' in error) {
            result.content = error.message
        }
    }

    return result
}

const notification = function(n, callback) {
    let notification = n

    if (typeof n === "string") {
        notification = {
            content: notification,
            type: 'info'
        }
    }

    if (!notification.title) {
        switch (notification.type) {
            case 'error':
                notification.title = trans('main.error')
                break
            case 'success':
                notification.title = trans('main.success')
                break
            default:
                notification.title = trans('main.info')
                break
        }
    }

    switch (notification.type) {
        case 'error':
            window.$dialog.error({
                title: notification.title,
                content: notification.content,
                style: notification.style,
                maskClosable: notification.maskClosable ?? true,
                positiveText: trans('main.close'),
                closable: false,
                onMaskClick: notification.maskClosable ?? (() => {
                    if (typeof callback === "function") {
                        callback();
                    }
                }),
                onPositiveClick: () => {
                    if (typeof callback === "function") {
                        callback();
                    }
                },
            });
            break
        case 'success':
            window.$dialog.success({
                title: notification.title,
                content: notification.content,
                style: notification.style,
                maskClosable: notification.maskClosable ?? true,
                positiveText: trans('main.close'),
                closable: false,
                onMaskClick: notification.maskClosable ?? (() => {
                    if (typeof callback === "function") {
                        callback();
                    }
                }),
                onPositiveClick: () => {
                    if (typeof callback === "function") {
                        callback();
                    }
                },
            });
            break
        default:
            window.$dialog.info({
                title: notification.title,
                content: notification.content,
                style: notification.style,
                maskClosable: notification.maskClosable ?? true,
                positiveText: trans('main.close'),
                closable: false,
                onMaskClick: notification.maskClosable ?? (() => {
                    if (typeof callback === "function") {
                        callback();
                    }
                }),
                onPositiveClick: () => {
                    if (typeof callback === "function") {
                        callback();
                    }
                },
            });
            break
    }
}

const alert = function(message, callback, content = "") {
    window.$dialog.info({
        title: message,
        content: content,
        positiveText: trans('main.close'),
        closable: false,
        onPositiveClick: () => {
            if (typeof callback === "function") {
                callback();
            }
        },
    });
}

let actionConfirmed = false;
const confirmAction = (e, message) => {
    if (!actionConfirmed) {
        e.preventDefault();

        confirm(message, () => {
            const clonedEvent = new e.constructor(e.type, e);
            e.target.dispatchEvent(clonedEvent);
        });
    }
}

const confirm = (message, callback) => {
    window.$dialog.success({
        title: message,
        content: "",
        positiveText: trans('main.yes'),
        negativeText: trans('main.no'),
        closable: false,
        onPositiveClick: () => {
            actionConfirmed = true
            if (typeof callback === "function") {
                callback();
            }
        },
        onNegativeClick: () => {
            actionConfirmed = false
        }
    });
}

export {errorNotification, notification, alert, confirmAction, confirm}

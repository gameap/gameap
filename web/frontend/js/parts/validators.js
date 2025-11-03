import {trans} from "@/i18n/i18n";
import _ from 'lodash';

const requiredValidator = (label) => {
    return (rule, value) => {
        if (!value) {
            return new Error(
                trans('validation.required', { attribute: label }),
            )
        }
    };
}

const stringLengthValidator = (label, min, max) => {
    return (rule, value) => {
        if (value.length < min || value.length > max) {
            return new Error(
                trans(
                    'validation.between.string',
                    { attribute: label, min: min, max: max },
                ),
            )
        }
    }
}

const stringMinLengthValidator = (label, min) => {
    return (rule, value) => {
        if (value.length < min) {
            return new Error(
                trans(
                    'validation.min.string',
                    { attribute: label, min: min},
                ),
            )
        }
    }
}

const sameWithValidator = (label, targetLabel, targetValueReader) => {
    return (rule, value) => {
        if (!_.isEqual(value, targetValueReader())) {
            return new Error(
                trans(
                    'validation.same',
                    { attribute: label, other: targetLabel },
                ),
            )
        }
    }
}

const isArrayNotEmptyValidator = (label) => {
    return (rule, value) => {
        if (_.isEmpty(value)) {
            return new Error(
                trans(
                    'validation.gte.array',
                    { attribute: label },
                )
            )
        }
    }
}

const allOfValidator = (...validators) => {
    return (rule, value) => {
        for (const validator of validators) {
            const error = validator(rule, value)
            if (error) {
                return error
            }
        }
    }
}

const ifNotEmptyValidator = (...validators) => {
    return (rule, value) => {
        if (!_.isEmpty(value)) {
            for (const validator of validators) {
                const error = validator(rule, value)
                if (error) {
                    return error
                }
            }
        }
    }
}

export {
    // common
    requiredValidator,

    // string
    stringLengthValidator,
    stringMinLengthValidator,
    sameWithValidator,

    // array
    isArrayNotEmptyValidator,

    // special
    allOfValidator,
    ifNotEmptyValidator,
}
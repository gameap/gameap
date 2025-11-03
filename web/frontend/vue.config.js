const chainWebpack = (webpackConfig) => {
    webpackConfig.resolve.alias.set('vue', '@vue/compat');
    webpackConfig.module
        .rule('vue')
        .use('vue-loader')
        .tap((options) => ({
            ...options,
            compilerOptions: {
                compatConfig: {
                    MODE: 2,
                },
            },
        }));
};

module.exports = {
    chainWebpack,
};

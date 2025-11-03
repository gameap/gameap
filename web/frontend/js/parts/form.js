window.addEventListener('load', function() {
    document.querySelectorAll('.custom-file-input').forEach(function(input) {
        input.addEventListener('change', function() {
            this.nextElementSibling.innerHTML = this.value.replace(/^.*\\/, '');
        });
    });
});

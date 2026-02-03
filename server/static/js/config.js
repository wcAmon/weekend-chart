// Base path for the application
// Use /weekend-chart prefix for bookbark.io, empty for wake.bookbark.io
const BASE_PATH = window.location.hostname === 'wake.bookbark.io' ? '' : '/weekend-chart';

function apiUrl(path) {
    return BASE_PATH + path;
}

function pageUrl(path) {
    return BASE_PATH + path;
}

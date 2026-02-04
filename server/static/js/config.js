// Base path for the application
// Direct hosts (no prefix): wake.bookbark.io, wake.loader.land
// With /weekend-chart prefix: bookbark.io (via Caddy reverse proxy)
const DIRECT_HOSTS = ['wake.bookbark.io', 'wake.loader.land', 'localhost'];
const BASE_PATH = DIRECT_HOSTS.includes(window.location.hostname) ? '' : '/weekend-chart';

function apiUrl(path) {
    return BASE_PATH + path;
}

function pageUrl(path) {
    return BASE_PATH + path;
}

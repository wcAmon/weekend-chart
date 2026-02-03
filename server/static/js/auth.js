// Check if already logged in
fetch(apiUrl('/api/check-auth'))
    .then(r => r.json())
    .then(data => {
        if (data.authenticated) {
            window.location.href = pageUrl('/dashboard.html');
        }
    });

// Login form handler
document.getElementById('loginForm').addEventListener('submit', async (e) => {
    e.preventDefault();

    const username = document.getElementById('username').value;
    const password = document.getElementById('password').value;
    const errorMsg = document.getElementById('errorMsg');

    errorMsg.classList.add('hidden');

    try {
        const response = await fetch(apiUrl('/api/login'), {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ username, password })
        });

        const data = await response.json();

        if (data.success) {
            window.location.href = pageUrl('/dashboard.html');
        } else {
            errorMsg.textContent = data.message || '登入失敗';
            errorMsg.classList.remove('hidden');
        }
    } catch (err) {
        errorMsg.textContent = '網路錯誤，請稍後再試';
        errorMsg.classList.remove('hidden');
    }
});

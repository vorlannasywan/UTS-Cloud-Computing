// index.js (Frontend Express)
const express = require('express');
const multer = require('multer');
const axios = require('axios');
const session = require('express-session');
const methodOverride = require('method-override');
const fs = require('fs');
const FormData = require('form-data');
const path = require('path');

const app = express();
const upload = multer({ dest: 'uploads/' });

const backendUrl = 'http://10.0.2.43:8080'; // Ganti dengan IP backend Go

app.set('view engine', 'ejs');
app.use(express.static('public'));
app.use(express.urlencoded({ extended: true }));
app.use(express.json());
app.use(methodOverride('_method'));

app.use(session({
  secret: 'secret_key',
  resave: false,
  saveUninitialized: false
}));

app.get('/', async (req, res) => {
  try {
    const response = await axios.get(`${backendUrl}/api/products`);
    res.render('index', { products: response.data, user: req.session.user });
  } catch (error) {
    console.error('Gagal mengambil produk:', error.message);
    res.render('index', { products: [], user: req.session.user });
  }
});

app.get('/edit', async (req, res) => {
  if (!req.session.user) {
    return res.redirect('/login');
  }
  try {
    const response = await axios.get(`${backendUrl}/api/products`);
    res.render('edit', { products: response.data, user: req.session.user });
  } catch (error) {
    console.error('Gagal mengambil produk:', error.message);
    res.render('edit', { products: [], user: req.session.user });
  }
});

app.get('/login', (req, res) => {
  res.render('login', { error: null });
});

app.post('/login', async (req, res) => {
  try {
    const response = await axios.post(`${backendUrl}/api/login`, {
      username: req.body.username,
      password: req.body.password
    });
    req.session.user = { username: req.body.username, token: response.data.token };
    res.redirect('/edit');
  } catch (error) {
    console.error('Login gagal:', error.message);
    res.render('login', { error: 'Login gagal' });
  }
});

app.post('/products', upload.single('image'), async (req, res) => {
  if (!req.session.user) {
    return res.status(401).send('Login diperlukan');
  }
  try {
    // Upload gambar ke backend
    const imageForm = new FormData();
    imageForm.append('file', fs.createReadStream(req.file.path));

    const uploadResponse = await axios.post(`${backendUrl}/api/upload`, imageForm, {
      headers: {
        Authorization: `Bearer ${req.session.user.token}`,
        ...imageForm.getHeaders()
      }
    });

    const imageUrl = uploadResponse.data.image_url;

    // Simpan produk
    await axios.post(`${backendUrl}/api/products`, {
      name: req.body.name,
      price: req.body.price,
      image_url: imageUrl
    }, {
      headers: { Authorization: `Bearer ${req.session.user.token}` }
    });

    fs.unlinkSync(req.file.path); // Hapus file lokal setelah upload
    res.redirect('/edit');
  } catch (error) {
    console.error('Gagal menambah produk:', error.response?.data || error.message);
    res.status(500).send('Gagal menambah produk');
  }
});

app.put('/products/:id', async (req, res) => {
  if (!req.session.user) {
    return res.status(401).send('Login diperlukan');
  }
  try {
    await axios.put(`${backendUrl}/api/products/${req.params.id}`, {
      name: req.body.name,
      price: req.body.price,
      image_url: req.body.image_url
    }, {
      headers: { Authorization: `Bearer ${req.session.user.token}` }
    });
    res.redirect('/edit');
  } catch (error) {
    console.error('Gagal memperbarui produk:', error.response?.data || error.message);
    res.status(500).send('Gagal memperbarui produk');
  }
});

app.post('/products/:id/delete', async (req, res) => {
  if (!req.session.user) {
    return res.status(401).send('Login diperlukan');
  }
  try {
    await axios.delete(`${backendUrl}/api/products/${req.params.id}`, {
      headers: { Authorization: `Bearer ${req.session.user.token}` }
    });
    res.redirect('/edit');
  } catch (error) {
    console.error('Gagal menghapus produk:', error.response?.data || error.message);
    res.status(500).send('Gagal menghapus produk');
  }
});

app.get('/logout', (req, res) => {
  req.session.destroy();
  res.redirect('/login');
});

app.listen(3000, () => {
  console.log('Frontend berjalan di port 3000');
});

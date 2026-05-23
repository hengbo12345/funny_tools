import { elements, addConsoleLine } from './app.js';

export let map = null;
export let marker = null;
const defaultCoords = [39.9042, 116.4074];

export function initMap() {
	map = L.map('map').setView(defaultCoords, 13);

	L.tileLayer('https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png', {
		attribution: '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> contributors &copy; <a href="https://carto.com/attributions">CARTO</a>',
		subdomains: 'abcd',
		maxZoom: 20
	}).addTo(map);

	const customIcon = L.divIcon({
		className: 'custom-div-icon',
		html: `<div style="
			background-color: #00f2fe; 
			width: 14px; 
			height: 14px; 
			border-radius: 50%; 
			border: 2px solid #fff;
			box-shadow: 0 0 10px rgba(0,242,254,0.8);
		"></div>`,
		iconSize: [14, 14],
		iconAnchor: [7, 7]
	});

	marker = L.marker(defaultCoords, {
		draggable: true,
		icon: customIcon
	}).addTo(map);

	// Sync input coordinates when dragging marker
	marker.on('dragend', () => {
		const latLng = marker.getLatLng();
		elements.inputLat.value = latLng.lat.toFixed(6);
		elements.inputLng.value = latLng.lng.toFixed(6);
		elements.overlayLat.textContent = latLng.lat.toFixed(6);
		elements.overlayLng.textContent = latLng.lng.toFixed(6);
	});

	// Sync input coordinates on clicking map
	map.on('click', (e) => {
		const latLng = e.latlng;
		marker.setLatLng(latLng);
		elements.inputLat.value = latLng.lat.toFixed(6);
		elements.inputLng.value = latLng.lng.toFixed(6);
		elements.overlayLat.textContent = latLng.lat.toFixed(6);
		elements.overlayLng.textContent = latLng.lng.toFixed(6);
	});
}

export async function searchAddress() {
	const query = elements.addressSearch.value.trim();
	if (!query) return;

	elements.btnSearch.disabled = true;
	elements.btnSearch.innerHTML = '...';

	try {
		const response = await fetch(`https://nominatim.openstreetmap.org/search?format=json&q=${encodeURIComponent(query)}&limit=5`, {
			headers: {
				'Accept-Language': 'zh-CN,zh;q=0.9'
			}
		});
		
		if (!response.ok) throw new Error("HTTP request failed");
		const results = await response.json();
		
		elements.searchResults.innerHTML = '';
		if (results.length === 0) {
			elements.searchResults.innerHTML = '<li class="no-results">未找到匹配地址</li>';
			elements.searchResults.classList.remove('hidden');
			return;
		}

		results.forEach(item => {
			const li = document.createElement('li');
			li.textContent = item.display_name;
			li.addEventListener('click', () => {
				const lat = parseFloat(item.lat);
				const lng = parseFloat(item.lon);
				
				// Update marker & inputs
				marker.setLatLng([lat, lng]);
				map.setView([lat, lng], 14);
				elements.inputLat.value = lat.toFixed(6);
				elements.inputLng.value = lng.toFixed(6);
				elements.overlayLat.textContent = lat.toFixed(6);
				elements.overlayLng.textContent = lng.toFixed(6);
				
				elements.searchResults.classList.add('hidden');
				elements.addressSearch.value = item.display_name;
				addConsoleLine(`📍 地图选点已切换至: ${item.display_name} (${lat.toFixed(4)}, ${lng.toFixed(4)})`, "system");
			});
			elements.searchResults.appendChild(li);
		});
		elements.searchResults.classList.remove('hidden');
	} catch (err) {
		addConsoleLine(`❌ Nominatim 选点检索发生网络异常: ${err.message}`, "error");
	} finally {
		elements.btnSearch.disabled = false;
		elements.btnSearch.innerHTML = `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
			<circle cx="11" cy="11" r="8"></circle>
			<line x1="21" y1="21" x2="16.65" y2="16.65"></line>
		</svg>`;
	}
}

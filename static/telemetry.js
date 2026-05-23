export function updateTelemetryBoard(data) {
	const rxFixType = document.getElementById('rx-fix-type');
	const rxAccuracy = document.getElementById('rx-accuracy');
	const rxCoords = document.getElementById('rx-coords');
	const rxAltitude = document.getElementById('rx-altitude');
	const rxTime = document.getElementById('rx-time');
	const rxTtff = document.getElementById('rx-ttff');
	const rxSatsUsed = document.getElementById('rx-sats-used');
	const rxSatsInView = document.getElementById('rx-sats-inview');
	const badgeGps = document.getElementById('badge-gps');
	const badgeBds = document.getElementById('badge-bds');

	if (!rxFixType) return;

	rxFixType.textContent = data.fix_type.toUpperCase() + (data.fix_type === 'Searching' ? '...' : '');
	rxFixType.className = 'telemetry-value';
	if (data.fix_type === 'Searching') {
		rxFixType.classList.add('text-searching');
	} else if (data.fix_type === '2D Fix') {
		rxFixType.classList.add('text-2dfix');
	} else if (data.fix_type === '3D Fix') {
		rxFixType.classList.add('text-3dfix');
	}

	if (data.fix_type !== 'Searching') {
		rxCoords.textContent = `${data.lat.toFixed(6)} , ${data.lng.toFixed(6)}`;
		rxAltitude.textContent = `海拔: ${data.alt.toFixed(1)} m`;
		rxAccuracy.textContent = `精度: ±${data.accuracy.toFixed(1)}m`;
	} else {
		rxCoords.textContent = '-- , --';
		rxAltitude.textContent = '海拔: --';
		rxAccuracy.textContent = '精度: --';
	}

	if (data.utc_time) {
		const parts = data.utc_time.split('T');
		if (parts.length > 1) {
			rxTime.textContent = parts[1].replace('Z', '');
		} else {
			rxTime.textContent = data.utc_time;
		}
	}
	rxTtff.textContent = `TTFF (冷启动): ${data.ttff.toFixed(1)} s`;

	rxSatsUsed.textContent = data.sats_used;
	rxSatsInView.textContent = data.sats_in_view;

	let hasGps = false;
	let hasBds = false;
	if (data.satellites && data.satellites.length > 0) {
		data.satellites.forEach(s => {
			if (s.used) {
				if (s.system === 'gps') hasGps = true;
				if (s.system === 'beidou') hasBds = true;
			}
		});
	}

	if (hasGps) {
		badgeGps.className = "sat-badge gps-badge active";
	} else {
		badgeGps.className = "sat-badge gps-badge inactive";
	}

	if (hasBds) {
		badgeBds.className = "sat-badge bds-badge active";
	} else {
		badgeBds.className = "sat-badge bds-badge inactive";
	}
}

export function updateSNRBars(sats) {
	const container = document.getElementById('snr-bars-container');
	if (!container) return;

	if (!sats || sats.length === 0) {
		container.innerHTML = '<div class="snr-placeholder">等待接收解算数据...</div>';
		return;
	}

	const sorted = [...sats].sort((a, b) => a.prn.localeCompare(b.prn));
	
	container.innerHTML = '';
	sorted.forEach(sat => {
		const row = document.createElement('div');
		row.className = 'snr-bar-row';

		const prnDiv = document.createElement('div');
		prnDiv.className = `snr-bar-prn ${sat.system}`;
		prnDiv.textContent = sat.prn;

		const barOuter = document.createElement('div');
		barOuter.className = 'snr-bar-outer';

		const barInner = document.createElement('div');
		barInner.className = `snr-bar-inner ${sat.system}`;
		if (!sat.used) {
			barInner.classList.add('unused');
		}
		const percent = Math.max(0, Math.min(100, ((sat.snr - 10) / 45) * 100));
		barInner.style.width = `${percent}%`;

		barOuter.appendChild(barInner);

		const valDiv = document.createElement('div');
		valDiv.className = 'snr-bar-val';
		valDiv.textContent = Math.round(sat.snr);

		row.appendChild(prnDiv);
		row.appendChild(barOuter);
		row.appendChild(valDiv);

		container.appendChild(row);
	});
}

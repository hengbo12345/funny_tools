import { state } from './app.js';

export const resizeCanvases = () => {
	const list = [
		{ canvas: document.getElementById('canvas-spectrum'), width: 1.0 },
		{ canvas: document.getElementById('canvas-waterfall'), width: 1.0 },
		{ canvas: document.getElementById('canvas-skyplot'), width: 0.0 }
	];

	list.forEach(item => {
		if (!item.canvas) return;
		const rect = item.canvas.parentElement.getBoundingClientRect();
		if (item.width > 0) {
			item.canvas.width = rect.width * window.devicePixelRatio;
			item.canvas.height = (rect.height || 160) * window.devicePixelRatio;
		} else {
			const side = Math.min(rect.width, rect.height || 270);
			item.canvas.width = side * window.devicePixelRatio;
			item.canvas.height = side * window.devicePixelRatio;
		}
	});
};

window.addEventListener('resize', resizeCanvases);

export function drawSpectrum(low, high, width, dbs) {
	const canvas = document.getElementById('canvas-spectrum');
	if (!canvas) return;
	const ctx = canvas.getContext('2d');
	const w = canvas.width;
	const h = canvas.height;

	ctx.clearRect(0, 0, w, h);
	ctx.fillStyle = '#020306';
	ctx.fillRect(0, 0, w, h);

	const paddingLeft = 45 * window.devicePixelRatio;
	const paddingRight = 15 * window.devicePixelRatio;
	const paddingTop = 25 * window.devicePixelRatio;
	const paddingBottom = 25 * window.devicePixelRatio;

	const graphW = w - paddingLeft - paddingRight;
	const graphH = h - paddingTop - paddingBottom;

	// Draw Grid
	ctx.strokeStyle = 'rgba(255, 255, 255, 0.03)';
	ctx.lineWidth = 1 * window.devicePixelRatio;
	ctx.fillStyle = 'rgba(255, 255, 255, 0.3)';
	ctx.font = `${Math.round(9 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
	ctx.textAlign = 'right';
	ctx.textBaseline = 'middle';

	// dB Grid lines
	const minDB = -90;
	const maxDB = -30;
	const dbSteps = [-80, -70, -60, -50, -40];
	
	dbSteps.forEach(db => {
		const y = paddingTop + graphH * (1 - (db - minDB) / (maxDB - minDB));
		ctx.beginPath();
		ctx.moveTo(paddingLeft, y);
		ctx.lineTo(w - paddingRight, y);
		ctx.stroke();
		ctx.fillText(db + ' dB', paddingLeft - 8 * window.devicePixelRatio, y);
	});

	// Frequency markers
	const numFreqSteps = 5;
	ctx.textAlign = 'center';
	ctx.textBaseline = 'top';
	for (let i = 0; i < numFreqSteps; i++) {
		const frac = i / (numFreqSteps - 1);
		const freq = low + frac * (high - low);
		const x = paddingLeft + frac * graphW;
		const mhz = (freq / 1000000).toFixed(1);
		
		ctx.beginPath();
		ctx.moveTo(x, paddingTop);
		ctx.lineTo(x, h - paddingBottom);
		ctx.stroke();
		
		ctx.fillText(mhz + ' MHz', x, h - paddingBottom + 6 * window.devicePixelRatio);
	}

	// Draw B1I and L1 markers
	const b1iFreq = 1561098000;
	const l1Freq = 1575420000;

	const drawMarker = (freq, label, color) => {
		if (freq >= low && freq <= high) {
			const frac = (freq - low) / (high - low);
			const x = paddingLeft + frac * graphW;
			ctx.strokeStyle = color;
			ctx.lineWidth = 1 * window.devicePixelRatio;
			ctx.setLineDash([4 * window.devicePixelRatio, 4 * window.devicePixelRatio]);
			ctx.beginPath();
			ctx.moveTo(x, paddingTop);
			ctx.lineTo(x, h - paddingBottom);
			ctx.stroke();
			ctx.setLineDash([]);

			ctx.fillStyle = color;
			ctx.font = `bold ${Math.round(8 * window.devicePixelRatio)}px "Montserrat", sans-serif`;
			ctx.textAlign = 'center';
			ctx.fillText(label, x, paddingTop - 12 * window.devicePixelRatio);
		}
	};

	drawMarker(b1iFreq, 'BDS B1I', '#10b981');
	drawMarker(l1Freq, 'GPS L1 / BDS B1C', '#00f2fe');

	// Plot spectrum line
	if (dbs && dbs.length > 0) {
		ctx.beginPath();
		ctx.lineWidth = 2 * window.devicePixelRatio;
		
		ctx.shadowBlur = 6 * window.devicePixelRatio;
		ctx.shadowColor = '#00f2fe';

		const grad = ctx.createLinearGradient(paddingLeft, 0, w - paddingRight, 0);
		grad.addColorStop(0, '#4facfe');
		grad.addColorStop(1, '#00f2fe');
		ctx.strokeStyle = grad;

		for (let i = 0; i < dbs.length; i++) {
			const frac = i / (dbs.length - 1);
			const x = paddingLeft + frac * graphW;
			const db = Math.max(minDB, Math.min(maxDB, dbs[i]));
			const y = paddingTop + graphH * (1 - (db - minDB) / (maxDB - minDB));

			if (i === 0) {
				ctx.moveTo(x, y);
			} else {
				ctx.lineTo(x, y);
			}
		}
		ctx.stroke();
		ctx.shadowBlur = 0; // reset shadow
	}
}

export function drawWaterfall(dbs) {
	const canvas = document.getElementById('canvas-waterfall');
	if (!canvas) return;
	const ctx = canvas.getContext('2d');
	const w = canvas.width;
	const h = canvas.height;

	const paddingLeft = 45 * window.devicePixelRatio;
	const paddingRight = 15 * window.devicePixelRatio;
	const paddingTop = 2 * window.devicePixelRatio;
	const paddingBottom = 2 * window.devicePixelRatio;

	const graphW = w - paddingLeft - paddingRight;
	const graphH = h - paddingTop - paddingBottom;

	// Push new sweep line to history in state
	state.waterfallLines.unshift(dbs);
	if (state.waterfallLines.length > state.maxWaterfallLines) {
		state.waterfallLines.pop();
	}

	ctx.fillStyle = '#020306';
	ctx.fillRect(0, 0, w, h);

	const rowH = graphH / state.maxWaterfallLines;
	
	for (let r = 0; r < state.waterfallLines.length; r++) {
		const line = state.waterfallLines[r];
		const y = paddingTop + r * rowH;

		const cellW = graphW / line.length;
		
		for (let i = 0; i < line.length; i++) {
			const db = line[i];
			const x = paddingLeft + i * cellW;

			let color;
			const normalized = Math.max(0, Math.min(1, (db + 80) / 45));

			if (normalized < 0.3) {
				const ratio = normalized / 0.3;
				const red = Math.round(2 * ratio);
				const green = Math.round(15 * ratio + 3);
				const blue = Math.round(80 + 175 * ratio);
				color = `rgb(${red},${green},${blue})`;
			} else if (normalized < 0.6) {
				const ratio = (normalized - 0.3) / 0.3;
				const red = 2;
				const green = Math.round(18 + 160 * ratio);
				const blue = Math.round(255 - 200 * ratio);
				color = `rgb(${red},${green},${blue})`;
			} else if (normalized < 0.85) {
				const ratio = (normalized - 0.6) / 0.25;
				const red = Math.round(16 + 220 * ratio);
				const green = Math.round(178 + 40 * ratio);
				const blue = Math.round(55 - 45 * ratio);
				color = `rgb(${red},${green},${blue})`;
			} else {
				const ratio = (normalized - 0.85) / 0.15;
				const red = 255;
				const green = Math.round(218 - 180 * ratio);
				const blue = 10;
				color = `rgb(${red},${green},${blue})`;
			}

			ctx.fillStyle = color;
			ctx.fillRect(x, y, cellW + 1, rowH + 1);
		}
	}

	// Draw scale axis
	ctx.strokeStyle = 'rgba(255, 255, 255, 0.05)';
	ctx.lineWidth = 1 * window.devicePixelRatio;
	ctx.beginPath();
	ctx.moveTo(paddingLeft, paddingTop);
	ctx.lineTo(paddingLeft, h - paddingBottom);
	ctx.stroke();
}

export function drawSkyplot(sats) {
	const canvas = document.getElementById('canvas-skyplot');
	if (!canvas) return;
	const ctx = canvas.getContext('2d');
	const w = canvas.width;
	const h = canvas.height;

	ctx.clearRect(0, 0, w, h);
	ctx.fillStyle = 'rgba(2, 3, 6, 0.5)';
	ctx.fillRect(0, 0, w, h);

	const centerX = w / 2;
	const centerY = h / 2;
	const radius = Math.min(centerX, centerY) - 16 * window.devicePixelRatio;

	// Draw concentric circles
	ctx.strokeStyle = 'rgba(255, 255, 255, 0.035)';
	ctx.lineWidth = 1 * window.devicePixelRatio;
	
	const rings = [30, 60, 90];
	rings.forEach(elev => {
		const r = radius * (1 - elev / 90);
		ctx.beginPath();
		ctx.arc(centerX, centerY, r, 0, 2 * Math.PI);
		ctx.stroke();

		if (elev < 90) {
			ctx.fillStyle = 'rgba(255, 255, 255, 0.12)';
			ctx.font = `${Math.round(8 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
			ctx.fillText(elev + '°', centerX + 4 * window.devicePixelRatio, centerY - r - 2 * window.devicePixelRatio);
		}
	});

	// Direction grid lines
	ctx.beginPath();
	ctx.moveTo(centerX, centerY - radius);
	ctx.lineTo(centerX, centerY + radius);
	ctx.moveTo(centerX - radius, centerY);
	ctx.lineTo(centerX + radius, centerY);
	ctx.stroke();

	// Direction Labels
	ctx.fillStyle = 'rgba(255, 255, 255, 0.4)';
	ctx.font = `bold ${Math.round(9 * window.devicePixelRatio)}px "Montserrat", sans-serif`;
	ctx.textAlign = 'center';
	ctx.textBaseline = 'middle';
	
	ctx.fillText('N', centerX, centerY - radius - 8 * window.devicePixelRatio);
	ctx.fillText('S', centerX, centerY + radius + 8 * window.devicePixelRatio);
	ctx.fillText('E', centerX + radius + 8 * window.devicePixelRatio, centerY);
	ctx.fillText('W', centerX - radius - 8 * window.devicePixelRatio, centerY);

	// Render satellites
	if (sats && sats.length > 0) {
		sats.forEach(sat => {
			const r = radius * (1 - sat.elevation / 90);
			const angle = (sat.azimuth * Math.PI) / 180 - Math.PI / 2;
			const x = centerX + r * Math.cos(angle);
			const y = centerY + r * Math.sin(angle);

			const isBds = sat.system === 'beidou';
			const baseColor = isBds ? '#10b981' : '#00f2fe';

			if (sat.used) {
				ctx.shadowBlur = 8 * window.devicePixelRatio;
				ctx.shadowColor = baseColor;
				ctx.fillStyle = baseColor;
				ctx.beginPath();
				ctx.arc(x, y, 7 * window.devicePixelRatio, 0, 2 * Math.PI);
				ctx.fill();
				ctx.shadowBlur = 0; // reset
			} else {
				ctx.strokeStyle = baseColor;
				ctx.lineWidth = 1.5 * window.devicePixelRatio;
				ctx.fillStyle = 'rgba(13, 16, 27, 0.7)';
				ctx.beginPath();
				ctx.arc(x, y, 6 * window.devicePixelRatio, 0, 2 * Math.PI);
				ctx.fill();
				ctx.stroke();
			}

			ctx.fillStyle = sat.used ? '#06070b' : '#f3f4f6';
			ctx.font = `bold ${Math.round(8 * window.devicePixelRatio)}px "JetBrains Mono", monospace`;
			ctx.textAlign = 'center';
			ctx.textBaseline = 'middle';
			
			const prnNum = sat.prn.substring(1);
			if (sat.used) {
				ctx.fillText(prnNum, x, y);
			} else {
				ctx.fillStyle = 'rgba(243, 244, 246, 0.55)';
				ctx.fillText(sat.prn, x, y + 11 * window.devicePixelRatio);
			}
		});
	}
}

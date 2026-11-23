// PRATHAM HOSPITAL DIRECTORY - Frontend Client Script

document.addEventListener('DOMContentLoaded', function () {
  initGeographyCascading();
  initTableRowClicks();
  initSearchDebounce();
});

// Cascading Geography Filters (State -> District -> Location)
function initGeographyCascading() {
  const stateSelect = document.getElementById('filter-state');
  const districtSelect = document.getElementById('filter-district');
  const locationSelect = document.getElementById('filter-location');

  if (!stateSelect || !districtSelect || !locationSelect) return;

  stateSelect.addEventListener('change', function () {
    const selectedState = this.value;
    // When state changes, reset child district and location
    districtSelect.value = '';
    locationSelect.value = '';
    fetchGeoOptions(selectedState, '');
  });

  districtSelect.addEventListener('change', function () {
    const selectedState = stateSelect.value;
    const selectedDistrict = this.value;
    locationSelect.value = '';
    fetchGeoOptions(selectedState, selectedDistrict);
  });
}

function fetchGeoOptions(state, district) {
  const districtSelect = document.getElementById('filter-district');
  const locationSelect = document.getElementById('filter-location');

  let url = '/api/v1/geography/cascading?state=' + encodeURIComponent(state);
  if (district) {
    url += '&district=' + encodeURIComponent(district);
  }

  fetch(url)
    .then(res => res.json())
    .then(data => {
      if (data.districts && !district) {
        const currentDist = districtSelect.value;
        districtSelect.innerHTML = '<option value="">All Districts</option>';
        data.districts.forEach(d => {
          const opt = document.createElement('option');
          opt.value = d.name;
          opt.textContent = `${d.name} (${d.hospital_count})`;
          if (d.name === currentDist) opt.selected = true;
          districtSelect.appendChild(opt);
        });
      }

      if (data.locations) {
        const currentLoc = locationSelect.value;
        locationSelect.innerHTML = '<option value="">All Locations</option>';
        data.locations.forEach(l => {
          const opt = document.createElement('option');
          opt.value = l.name;
          opt.textContent = `${l.name} (${l.hospital_count})`;
          if (l.name === currentLoc) opt.selected = true;
          locationSelect.appendChild(opt);
        });
      }
    })
    .catch(err => console.error('Error fetching cascading geography:', err));
}

// Table row click navigation
function initTableRowClicks() {
  const rows = document.querySelectorAll('.clickable-row');
  rows.forEach(row => {
    row.addEventListener('click', function (e) {
      if (e.target.tagName.toLowerCase() === 'a' || e.target.tagName.toLowerCase() === 'button' || e.target.closest('a')) {
        return;
      }
      const href = this.dataset.href;
      if (href) {
        window.location.href = href;
      }
    });
  });
}

// Search debounce
function initSearchDebounce() {
  const searchInput = document.getElementById('global-search-input');
  if (!searchInput) return;

  let debounceTimer;
  searchInput.addEventListener('input', function () {
    clearTimeout(debounceTimer);
    debounceTimer = setTimeout(() => {
      // Trigger auto-filter if desired or allow user to hit Enter / click Search
    }, 400);
  });
}

// Remove filter chip
function removeFilter(key, val) {
  const url = new URL(window.location.href);
  if (key === 'type') {
    const types = url.searchParams.getAll('type').filter(t => t !== val);
    url.searchParams.delete('type');
    types.forEach(t => url.searchParams.append('type', t));
  } else {
    url.searchParams.delete(key);
  }
  url.searchParams.set('page', '1');
  window.location.href = url.toString();
}

function clearAllFilters() {
  const url = new URL(window.location.href);
  url.search = '';
  window.location.href = url.pathname;
}

// Toggle Hospital Expandable Details (inline accordion)
function toggleHospitalExpand(id) {
  var card = document.getElementById('hosp-card-' + id);
  var details = document.getElementById('hosp-details-' + id);
  var btn = document.getElementById('info-btn-' + id);
  if (!details || !card) return;

  var isExpanded = details.style.display !== 'none';
  if (isExpanded) {
    details.style.display = 'none';
    card.classList.remove('expanded');
    if (btn) {
      btn.setAttribute('aria-expanded', 'false');
      btn.innerHTML = '<span class="info-icon-char">ⓘ</span>';
    }
  } else {
    details.style.display = 'block';
    card.classList.add('expanded');
    if (btn) {
      btn.setAttribute('aria-expanded', 'true');
      btn.innerHTML = '<span class="info-icon-char">✕</span>';
    }
  }
}



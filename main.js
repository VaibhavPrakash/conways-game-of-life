document.addEventListener("DOMContentLoaded", function () {
    var _a, _b;
    var size = 20;
    var grid = createGrid(size);
    var interval;
    (_a = document.getElementById('startButton')) === null || _a === void 0 ? void 0 : _a.addEventListener('click', function () {
        interval = setInterval(function () {
            grid = nextGeneration(grid);
            draw(grid);
        }, 100);
    });
    (_b = document.getElementById('stopButton')) === null || _b === void 0 ? void 0 : _b.addEventListener('click', function () {
        clearInterval(interval);
    });
    draw(grid);
});
function createGrid(size) {
    var grid = [];
    for (var i = 0; i < size; i++) {
        var row = [];
        for (var j = 0; j < size; j++) {
            row.push(false);
        }
        grid.push(row);
    }
    // Initialize some live cells
    grid[5][5] = true;
    grid[5][6] = true;
    grid[5][7] = true;
    return grid;
}
function nextGeneration(grid) {
    var size = grid.length;
    var newGrid = createGrid(size);
    for (var i = 0; i < size; i++) {
        for (var j = 0; j < size; j++) {
            var neighbors = countNeighbors(grid, i, j);
            if (grid[i][j] && (neighbors === 2 || neighbors === 3)) {
                newGrid[i][j] = true;
            }
            else if (!grid[i][j] && neighbors === 3) {
                newGrid[i][j] = true;
            }
        }
    }
    return newGrid;
}
function countNeighbors(grid, x, y) {
    var count = 0;
    for (var i = -1; i <= 1; i++) {
        for (var j = -1; j <= 1; j++) {
            if (i === 0 && j === 0)
                continue;
            var xi = (x + i + grid.length) % grid.length;
            var yj = (y + j + grid.length) % grid.length;
            if (grid[xi][yj])
                count++;
        }
    }
    return count;
}
function draw(grid) {
    var container = document.getElementById("grid-container");
    container.innerHTML = '';
    grid.forEach(function (row) {
        row.forEach(function (cell) {
            var cellDiv = document.createElement('div');
            cellDiv.className = 'cell';
            if (cell)
                cellDiv.classList.add('alive');
            container.appendChild(cellDiv);
        });
    });
}

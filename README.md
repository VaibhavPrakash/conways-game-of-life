# Conway's Game of Life

## Introduction

Conway's Game of Life is a cellular automaton invented by mathematician John Conway. It's a zero-player game, meaning that its evolution is determined by its initial state, requiring no further input. The game consists of a grid of cells, each of which can be alive or dead, and they evolve through generations following a set of rules.

## Installation

Follow these steps to clone the GitHub repository and run the game on your machine:

1. Open your terminal or command prompt.
2. Clone the repository using the command: `git clone https://github.com/VaibhavPrakash/conways-game-of-life.git`
3. Navigate to the directory: `cd conways-game-of-life`
4. Install dependencies (if required): `npm install`
5. Start the server using the command: `npm start`
6. Open your browser and navigate to `http://localhost:3000` (or the URL provided in the terminal) to view the game.

Alternatively, you can simply open `index.html` in your preferred browser.

Note: Make sure you have Node.js and npm installed on your system to run the server.


## How to Play

You can interact with the Game of Life through the following controls:

- **Start Button**: Begins the simulation of generations.
- **Stop Button**: Pauses the simulation.
- **Next Step Button**: Advances the grid to the next generation.
- **Reset Button**: Resets the grid to a new random pattern.

## Rules of the Cells

The evolution of the cells is determined by the following rules:

1. **Birth**: A dead cell with exactly 3 live neighbors becomes a live cell.
2. **Survival**: A live cell with 2 or 3 live neighbors stays alive.
3. **Death**: A live cell with fewer than 2 or more than 3 live neighbors dies.

These simple rules lead to a wide variety of patterns and behaviors, making Conway's Game of Life a fascinating subject to explore.

## License

This project is licensed under the MIT License.

## Contributing

If you would like to contribute to this project, please feel free to fork the repository and submit a pull request, or open an issue.

Happy exploring!

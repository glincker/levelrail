FROM node:22-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund
COPY app ./app
COPY MARKER ./MARKER
RUN npm run build && chown -R node:node /app
USER node
EXPOSE 3000
CMD ["npm", "start"]

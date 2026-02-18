FROM node:latest
RUN apt-get update && apt-get install -y curl
ADD . /app
WORKDIR /app
CMD ["node", "app.js"]
